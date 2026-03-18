package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// WARNING: This is an EXPERIMENTAL file created for demonstration purposes.
// MongoDB sessions and the sessCtx derived from them are NOT thread-safe for parallel use.
// Using goroutines inside a transaction using the same session will likely result in:
// "session is in use" errors or unpredictable behavior.

const (
	mongoURI = "mongodb://localhost:27017/"
	dbName   = "demo_db"
	collName = "goroutine_test"
	port     = "8081"
)

func main() {
	client, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/test-parallel-txn", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Use POST", http.StatusMethodNotAllowed)
			return
		}

		// 1. Simulating reading a file (we'll just use the size for demo)
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "File required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		_, _ = io.ReadAll(file) // Just consuming the stream

		// 2. Start Session & Transaction
		session, err := client.StartSession()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer session.EndSession(r.Context())

		fmt.Println("Starting transaction...")

		_, err = session.WithTransaction(r.Context(), func(sessCtx context.Context) (interface{}, error) {
			coll := client.Database(dbName).Collection(collName)

			var wg sync.WaitGroup
			errChan := make(chan error, 4)

			// 3. TRYING TO RUN 4 BATCHES IN PARALLEL (NOT RECOMMENDED)
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func(batchID int) {
					defer wg.Done()

					fmt.Printf("Batch %d: Attempting insert...\n", batchID)

					// Creating data for this batch
					docs := []interface{}{}
					for j := 0; j < 100; j++ {
						docs = append(docs, bson.M{"batch": batchID, "row": j, "ts": time.Now()})
					}

					// !!! USING THE SAME sessCtx IN PARALLEL !!!
					// This is where the "session is in use" error usually happens.
					_, err := coll.InsertMany(sessCtx, docs)
					if err != nil {
						fmt.Printf("Batch %d FAILED: %v\n", batchID, err)
						errChan <- err
						return
					}
					fmt.Printf("Batch %d: Success!\n", batchID)
				}(i)
			}

			wg.Wait()
			close(errChan)

			// Check if any goroutine failed
			for e := range errChan {
				if e != nil {
					return nil, e // This will trigger a rollback
				}
			}

			return nil, nil
		})

		if err != nil {
			fmt.Fprintf(w, "Transaction Global Error: %v\n", err)
			return
		}

		fmt.Fprintf(w, "Transaction Committed Successfully! (Surprisingly)\n")
	})

	fmt.Printf("Demo server starting on :%s\n", port)
	fmt.Printf("Endpoint: POST http://localhost:%s/test-parallel-txn\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
