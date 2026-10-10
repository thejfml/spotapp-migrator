package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Choice int

const (
	ChoiceYes Choice = iota
	ChoiceNo
	ChoiceAll
	ChoiceQuit
)

var reader = bufio.NewReader(os.Stdin)

// PromptPlaylist asks the user whether to migrate a specific playlist.
func PromptPlaylist(title string, trackCount int, owner string) (Choice, error) {
	fmt.Println("\n------------------------------------------------------------")
	fmt.Printf("🎵 Found Playlist: \033[1;36m%s\033[0m\n", title)
	fmt.Printf("   📊 Tracks: %d | Owner: %s\n", trackCount, owner)
	fmt.Println("------------------------------------------------------------")

	for {
		fmt.Print("Migrate this playlist? [\033[1;32my\033[0mes / \033[1;31mn\033[0mo / \033[1;33ma\033[0mll / \033[1;35mq\033[0muit]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return ChoiceQuit, err
		}
		input = strings.ToLower(strings.TrimSpace(input))

		switch input {
		case "y", "yes", "":
			return ChoiceYes, nil
		case "n", "no":
			return ChoiceNo, nil
		case "a", "all":
			return ChoiceAll, nil
		case "q", "quit", "exit":
			return ChoiceQuit, nil
		default:
			fmt.Println("Invalid input. Please enter 'y', 'n', 'a' (all), or 'q' (quit).")
		}
	}
}

// PromptInput requests a string input from the user with a prompt message.
func PromptInput(promptText string) string {
	fmt.Printf("%s: ", promptText)
	input, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(input)
}

// LogInfo prints a stylized info message.
func LogInfo(format string, a ...interface{}) {
	fmt.Printf("ℹ️  "+format+"\n", a...)
}

// LogSuccess prints a stylized success message.
func LogSuccess(format string, a ...interface{}) {
	fmt.Printf("✅ "+format+"\n", a...)
}

// LogWarning prints a stylized warning message.
func LogWarning(format string, a ...interface{}) {
	fmt.Printf("⚠️  "+format+"\n", a...)
}

// LogError prints a stylized error message.
func LogError(format string, a ...interface{}) {
	fmt.Printf("❌ "+format+"\n", a...)
}
