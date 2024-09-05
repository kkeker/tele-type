package main

import (
	"crypto/md5"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/go-vgo/robotgo"
)

var (
	filePath      string
	offsetPath    string
	base64Path    string
	encodedText   string
	currentIndex  int
	isPaused      bool
	startPrinting bool
	startTime     time.Time
	wg            sync.WaitGroup
)

// Progress bar structure
type PBar struct {
	Total      int
	DoneStr    string
	OngoingStr string
	Width      int
}

// Create a new progress bar
func NewPBar(total int) *PBar {
	return &PBar{
		Total:      total,
		DoneStr:    "#",
		OngoingStr: ".",
		Width:      50,
	}
}

// Render the progress bar with speed data
func (p *PBar) renderPBar(current int, speedSymbolsPerMin, speedBytesPerSec int, remainingTimeSeconds int) {
	progress := float64(current) / float64(p.Total)
	done := int(progress * float64(p.Width))

	bar := strings.Repeat(p.DoneStr, done) + strings.Repeat(p.OngoingStr, p.Width-done)

	hours := remainingTimeSeconds / 3600
	minutes := (remainingTimeSeconds % 3600) / 60
	seconds := remainingTimeSeconds % 60

	fmt.Printf("\r[%s] %.2f%% Speed: %d chars/min, Transfer rate: %d bytes/sec, Time remaining: %02d:%02d:%02d",
		bar, progress*100, speedSymbolsPerMin, speedBytesPerSec, hours, minutes, seconds)
}

func main() {
	// Welcome message
	fmt.Println("Welcome to TeleType v1!")

	// Define command line arguments
	fileFlag := flag.String("file", "", "Path to the input file (binary file)")
	flag.Parse()

	// Check if the file path is provided
	if *fileFlag == "" {
		fmt.Println("Usage: tele-type -file <path to input file>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Convert the path to an absolute path
	absFilePath, err := filepath.Abs(*fileFlag)
	if err != nil {
		log.Fatalf("Error getting absolute path: %v\n", err)
	}
	filePath = absFilePath

	// Calculate MD5 hash
	md5Sum, err := calculateMD5(filePath)
	if err != nil {
		log.Fatalf("Error calculating MD5: %v\n", err)
	}

	// Determine the size of the input file
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Fatalf("Error getting file information: %v\n", err)
	}
	fileSizeKB := float64(fileInfo.Size()) / 1024 // File size in kilobytes

	// Create full paths for Base64 and offset files
	base64Path = filePath + ".txt"
	offsetPath = filePath + ".offset"

	err = createBase64File(filePath, base64Path)
	if err != nil {
		log.Fatalf("Error creating Base64 file: %v\n", err)
	}

	// Determine the size of the Base64 file
	base64Info, err := os.Stat(base64Path)
	if err != nil {
		log.Fatalf("Error getting Base64 file information: %v\n", err)
	}
	base64FileSizeKB := float64(base64Info.Size()) / 1024 // Base64 file size in kilobytes

	// Display file information
	fmt.Printf("MD5 hash of the input file: %s\n", md5Sum)
	fmt.Printf("Path to the input file: %s\n", filePath)
	fmt.Printf("Input file size: %.2f KB\n", fileSizeKB)
	fmt.Printf("Full path to Base64 file: %s\n", base64Path)
	fmt.Printf("Base64 file size: %.2f KB\n", base64FileSizeKB)
	fmt.Printf("Full path to offset file: %s\n", offsetPath)

	// Read the Base64 file for printing simulation
	encodedText, err = readBase64File(base64Path)
	if err != nil {
		log.Fatalf("Error reading Base64 file: %v\n", err)
	}

	// Check for an existing offset file
	currentIndex = loadOffset(offsetPath)

	// Display hotkey hints
	fmt.Println("\nHotkeys:")
	fmt.Println("S - Start printing")
	fmt.Println("P - Pause/Resume printing")
	fmt.Println("Q - Cancel printing and exit")

	// Set up hotkeys
	go handleHotkeys()

	// Wait until printing starts
	for !startPrinting {
		time.Sleep(500 * time.Millisecond)
	}

	// Use WaitGroup to ensure all goroutines finish before exiting
	wg.Add(1)
	wg.Wait()
}

func calculateMD5(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Create Base64 file
func createBase64File(inputPath, outputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	base64Text := base64.StdEncoding.EncodeToString(data)
	return os.WriteFile(outputPath, []byte(base64Text), 0644)
}

// Read Base64 file
func readBase64File(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func printBase64Text() {
	totalSymbols := len(encodedText)
	startTime = time.Now()

	pBar := NewPBar(totalSymbols)

	for currentIndex < totalSymbols {
		if isPaused {
			fmt.Print("\rPrinting paused. Press P to continue.")
			time.Sleep(time.Second)
			continue
		}

		char := rune(encodedText[currentIndex])
		err := typeCharacter(char)
		if err != nil {
			log.Fatalf("Error simulating keystroke: %v\n", err)
		}

		currentIndex++
		saveOffset(offsetPath, currentIndex)

		elapsed := time.Since(startTime).Seconds()
		speedSymbolsPerMin := int(float64(currentIndex) / elapsed * 60)
		speedBytesPerSec := int(float64(currentIndex) / elapsed)
		remainingSymbols := totalSymbols - currentIndex
		remainingTimeSeconds := int(float64(remainingSymbols) / (float64(speedSymbolsPerMin) / 60))

		pBar.renderPBar(currentIndex, speedSymbolsPerMin, speedBytesPerSec, remainingTimeSeconds)
		time.Sleep(50 * time.Millisecond)
	}

	wg.Done()
}

// Simulate keypresses with special character handling using robotgo
func typeCharacter(char rune) error {
	switch char {
	case '+':
		robotgo.KeyTap("=", "shift") // '+' is shift + '='
	case '/':
		robotgo.KeyTap("/")
	case '=':
		robotgo.KeyTap("=")
	default:
		robotgo.TypeStr(string(char)) // Print regular characters
	}
	return nil
}

// Save offset
func saveOffset(filePath string, offset int) {
	os.WriteFile(filePath, []byte(fmt.Sprintf("%d", offset)), 0644)
}

// Load offset
func loadOffset(filePath string) int {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}
	var offset int
	fmt.Sscanf(string(data), "%d", &offset)
	return offset
}

func handleHotkeys() {
	for {
		char, key, err := keyboard.GetSingleKey()
		if err != nil {
			log.Fatalf("Error reading keys: %v\n", err)
		}

		switch char {
		case 's':
			if !startPrinting {
				fmt.Println("Printing started...")
				fmt.Println("Printing will start in 10 seconds...")
				for i := 10; i > 0; i-- {
					fmt.Printf("%d seconds remaining...\n", i)
					time.Sleep(1 * time.Second)
				}
				startPrinting = true
				go printBase64Text() // Start printing in a separate goroutine
			}
		case 'p':
			if !startPrinting {
				fmt.Println("Error: Printing has not started yet")
			} else {
				isPaused = !isPaused
				if isPaused {
					fmt.Print("\rPrinting paused                 ")
				} else {
					fmt.Print("\rResuming printing               ")
				}
			}
		case 'q':
			fmt.Println("Process cancelled")
			os.Remove(base64Path)
			os.Remove(offsetPath)
			os.Exit(0)
		}

		if key == keyboard.KeyEsc {
			break
		}
	}
}
