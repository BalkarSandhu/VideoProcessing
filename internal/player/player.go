package player

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Player handles video playback
type Player struct {
	reader *bufio.Reader
}

// New creates a new player instance
func New() *Player {
	return &Player{
		reader: bufio.NewReader(os.Stdin),
	}
}

// OfferPlayback asks user if they want to play the video
func (p *Player) OfferPlayback(outputPath string) error {
	fmt.Print("\n🎥 Would you like to play the processed video? (y/n): ")
	choice, _ := p.reader.ReadString('\n')
	choice = strings.TrimSpace(strings.ToLower(choice))

	if choice == "y" || choice == "yes" {
		return p.PlayVideo(outputPath)
	}

	return nil
}

// PlayVideo plays the specified video file
func (p *Player) PlayVideo(videoPath string) error {
	fmt.Printf("🎬 Opening video: %s\n", videoPath)

	// Check for available players
	players := []struct {
		cmd  string
		args []string
		name string
	}{
		{"ffplay", []string{"-autoexit", "-window_title", "Processed Video", videoPath}, "FFplay"},
		{"vlc", []string{"--intf", "dummy", "--play-and-exit", videoPath}, "VLC"},
		{"mpv", []string{"--really-quiet", videoPath}, "MPV"},
	}

	for _, player := range players {
		if _, err := exec.LookPath(player.cmd); err == nil {
			fmt.Printf("🎯 Using %s\n", player.name)

			cmd := exec.Command(player.cmd, player.args...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Start(); err != nil {
				fmt.Printf("❌ Failed to start %s: %v\n", player.name, err)
				continue
			}

			if player.cmd == "ffplay" {
				p.printFFplayControls()
			}

			if err := cmd.Wait(); err != nil {
				fmt.Printf("%s exited with error: %v\n", player.name, err)
			} else {
				fmt.Println("✅ Video playback finished")
			}

			return nil
		}
	}

	fmt.Println("❌ No video player found. Please install one of:")
	fmt.Println("   - FFmpeg (ffplay): https://ffmpeg.org/download.html")
	fmt.Println("   - VLC: https://www.videolan.org/vlc/")
	fmt.Println("   - MPV: https://mpv.io/")

	return fmt.Errorf("no video player available")
}

func (p *Player) printFFplayControls() {
	fmt.Println("🎮 FFplay controls:")
	fmt.Println("   Space: Pause/Play")
	fmt.Println("   ←/→: Seek ±10 seconds")
	fmt.Println("   ↑/↓: Seek ±1 minute")
	fmt.Println("   f: Toggle fullscreen")
	fmt.Println("   q: Quit")
}
