package main

import (
	"log"

	"csv-upload-parser/config"
	"csv-upload-parser/server"
)

func main() {
	cfg := config.Load("config/config.json")
	log.Println("CSV Upload Parser — config loaded")
	server.StartServer(cfg)
}
