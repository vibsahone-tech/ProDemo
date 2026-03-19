package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"csv-upload-parser/config"
	"csv-upload-parser/model"
	"csv-upload-parser/parser"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var (
	mongoClient *mongo.Client
	cfg         *config.Config
)

// uploadResponse is the JSON body returned for every upload request.
type uploadResponse struct {
	Message     string              `json:"message"`
	Inserted    int                 `json:"inserted"`
	Skipped     int                 `json:"skipped"`
	Groups      int                 `json:"groups"`
	ProtocolID  string              `json:"protocol_id,omitempty"`
	TotalTimeMs int64               `json:"total_time_ms"`
	DBTimeMs    int64               `json:"db_time_ms"`
	Errors      []parser.ParseError `json:"errors,omitempty"`
}

// protocolErrorResponse is returned when protocol form-data validation fails.
type protocolErrorResponse struct {
	Message string   `json:"message"`
	Errors  []string `json:"errors"`
}

// ── HTTP Handlers ───────────────────────────────────────────────────────────

// StartServer connects to MongoDB and starts the HTTP server.
func StartServer(c *config.Config) {
	cfg = c

	var err error
	mongoClient, err = mongo.Connect(options.Client().ApplyURI(cfg.DataStore.URL))
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mongoClient.Disconnect(ctx); err != nil {
			log.Println("MongoDB disconnect error:", err)
		}
	}()

	http.HandleFunc("/upload-csv", uploadCSV)
	log.Printf("Server starting on :%s  (max_rows=%d, max_upload=%dMB)\n",
		cfg.Server.Port, cfg.Upload.MaxRows, cfg.Server.MaxUploadSizeMB)
	log.Fatal(http.ListenAndServe(":"+cfg.Server.Port, nil))
}

func uploadCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, uploadResponse{Message: "method not allowed"})
		return
	}

	maxBytes := int64(cfg.Server.MaxUploadSizeMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	// Read all form data into memory at once, up to the configured limit.
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, uploadResponse{Message: "upload size exceeds limit: " + err.Error()})
		return
	}

	// ── Step 0: Start timer ───────────────────────────────────────────
	startTime := time.Now()

	// ── Step 1: Validate protocol form-data first ───────────────────────
	proto, protoErrs := parser.ParseProtocolForm(r)
	if len(protoErrs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, protocolErrorResponse{
			Message: "protocol validation failed",
			Errors:  protoErrs,
		})
		return
	}

	// ── Step 2: Read and parse the register CSV file ────────────────────
	file, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, uploadResponse{Message: "file upload error: " + err.Error()})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, uploadResponse{Message: "failed to read file: " + err.Error()})
		return
	}
	log.Printf("Uploaded file size: %d bytes\n", len(data))

	groups, registers, parseErrs, err := parser.ParseCSV(data, cfg.Upload.MaxRows)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, uploadResponse{Message: "CSV parse error: " + err.Error()})
		return
	}

	// Strict Validation: If even one row fails, abort the entire upload.
	if len(parseErrs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, uploadResponse{
			Message: "upload aborted: CSV contains validation errors. No records were inserted.",
			Skipped: len(parseErrs),
			Errors:  parseErrs,
		})
		return
	}

	// ── Step 3: Attach groups to protocol ───────────────────────────────
	proto.RegisterGroups = groups

	// Hardcode AuditInfo for this demo.
	now := time.Now()
	demoUserID := bson.NewObjectID()
	proto.AuditInfo = model.AuditInfo{
		CreatedBy: demoUserID,
		CreatedAt: now,
		UpdatedBy: demoUserID,
		UpdatedAt: now,
	}

	// ── Step 4: Execute transaction ─────────────────────────────────────
	dbStartTime := time.Now()
	insertErr := insertProtocolAndRegisters(r.Context(), proto, registers)
	dbDuration := time.Since(dbStartTime)

	if insertErr != nil {
		writeJSON(w, http.StatusInternalServerError, uploadResponse{
			Message:     "DB transaction failed: " + insertErr.Error(),
			Inserted:    0,
			Skipped:     0,
			TotalTimeMs: time.Since(startTime).Milliseconds(),
			DBTimeMs:    dbDuration.Milliseconds(),
		})
		return
	}

	writeJSON(w, http.StatusOK, uploadResponse{
		Message:     "upload successful: protocol and registers inserted atomically",
		Inserted:    len(registers),
		Groups:      len(groups),
		ProtocolID:  proto.ID.Hex(),
		TotalTimeMs: time.Since(startTime).Milliseconds(),
		DBTimeMs:    dbDuration.Milliseconds(),
	})
}

// insertProtocolAndRegisters inserts all registers and one protocol
// document inside a single MongoDB transaction.
func insertProtocolAndRegisters(ctx context.Context, proto model.Protocol, registers []model.Register) error {
	// Apply configurable transaction timeout.
	txnTimeout := time.Duration(cfg.DataStore.TransactionTimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, txnTimeout)
	defer cancel()

	session, err := mongoClient.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx context.Context) (interface{}, error) {
		regColl := mongoClient.Database(cfg.DataStore.DB).Collection(cfg.DataStore.RegisterCollection)
		protoColl := mongoClient.Database(cfg.DataStore.DB).Collection(cfg.DataStore.ProtocolCollection)

		// 1. Insert all registers at once.
		if len(registers) > 0 {
			docs := make([]interface{}, len(registers))
			for i, r := range registers {
				docs[i] = r
			}
			if _, err := regColl.InsertMany(sessCtx, docs); err != nil {
				return nil, err
			}
		}

		// 2. Insert protocol document.
		if _, err := protoColl.InsertOne(sessCtx, proto); err != nil {
			return nil, err
		}

		return nil, nil
	})

	return err
}

// ── Utility functions ───────────────────────────────────────────────────────

// writeJSON writes a JSON-encoded body with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
