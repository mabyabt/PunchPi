package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
)

func main() {
	// Create the Fyne application
	myApp := app.New()
	myApp.Settings().SetTheme(theme.DarkTheme())
	window := myApp.NewWindow("RFID Scanner")
	window.Resize(fyne.NewSize(400, 300))

	// Set up UI
	window.SetContent(createScannerScreen(window))

	window.ShowAndRun()
}
