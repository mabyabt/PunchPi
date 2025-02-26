package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var scanText *widget.Label
var lastScanTime time.Time

func createScannerScreen(window fyne.Window) fyne.CanvasObject {
	scanText = widget.NewLabel("Waiting for RFID card...")
	scanText.Alignment = fyne.TextAlignCenter

	scanEntry := widget.NewEntry()
	scanEntry.SetPlaceHolder("Scan RFID UID")

	scanEntry.OnChanged = func(uid string) {
		if time.Since(lastScanTime) < 1*time.Second {
			return
		}
		lastScanTime = time.Now()
		go sendScanToServer(uid)
		scanEntry.SetText("")
	}

	content := container.NewVBox(
		widget.NewLabelWithStyle("RFID Scanner", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		scanText,
		scanEntry,
	)
	return content
}

func sendScanToServer(uid string) {
	serverURL := "http://localhost:8080/scan"

	data := map[string]string{"uid": uid}
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Println("Failed to encode request:", err)
		return
	}

	resp, err := http.Post(serverURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println("Failed to send scan:", err)
		scanText.SetText("Server not reachable!")
		return
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK {
		scanText.SetText(string(body))
	} else {
		scanText.SetText("Error: " + string(body))
	}
}
