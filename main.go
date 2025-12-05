package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	imgcolor "image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fogleman/gg"
	"github.com/golang/geo/r3"
	ex "github.com/markus-wa/demoinfocs-golang/v5/examples"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"golang.org/x/sys/windows/registry"
)

const DOT_SIZE = 15

type PlayerData struct {
	Name            string `json:"name"`
	Health          int    `json:"health"`
	Armor           int    `json:"armor"`
	Money           int    `json:"money"`
	Kills           int    `json:"kills"`
	Deaths          int    `json:"deaths"`
	Assists         int    `json:"assists"`
	Team            int    `json:"team"`
	PrimaryWeapon   string `json:"primaryWeapon"`
	SecondaryWeapon string `json:"secondaryWeapon"`
	IsAlive         bool   `json:"isAlive"`
	HasKit          bool   `json:"hasKit"`
	HasBomb         bool   `json:"hasBomb"`
	IsPlanting      bool   `json:"isPlanting"`
	IsDefusing      bool   `json:"isDefusing"`
	IsScoped        bool   `json:"isScoped"`
	UserID          int    `json:"userId"` // For stable sorting
}

var (
	lastMapImg   []byte
	playersData  []PlayerData
	demoPath     string
	mapName      string
	initialTime  time.Time
	currentRound int

	// Console colors
	cyan    = color.New(color.FgCyan, color.Bold)
	green   = color.New(color.FgGreen, color.Bold)
	yellow  = color.New(color.FgYellow)
	red     = color.New(color.FgRed, color.Bold)
	magenta = color.New(color.FgMagenta, color.Bold)
	blue    = color.New(color.FgBlue)
	white   = color.New(color.FgWhite)
)

func main() {
	printHeader()

	// Auto-detect CS2 path
	cs2Path := detectCS2Path()

	reader := bufio.NewReader(os.Stdin)

	if cs2Path != "" {
		green.Printf("✓ CS2 detected: %s\n\n", cs2Path)
		yellow.Print("Use this path? (Y/n): ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "n" || response == "no" {
			cs2Path = ""
		}
	}

	if cs2Path == "" {
		yellow.Print("Enter CS2 path manually: ")
		cs2Path, _ = reader.ReadString('\n')
		cs2Path = strings.TrimSpace(cs2Path)
	}

	cs2Path = strings.ReplaceAll(cs2Path, "\\", "/")
	cs2Path = strings.ReplaceAll(cs2Path, "\"", "")

	// Demo filename input
	cyan.Print("\nDemo filename (e.g. 'radar' or 'radar.dem'): ")
	demoName, _ := reader.ReadString('\n')
	demoName = strings.TrimSpace(demoName)

	if !strings.HasSuffix(strings.ToLower(demoName), ".dem") {
		demoName += ".dem"
	}

	demoPath = filepath.Join(cs2Path, demoName)
	demoPath = strings.ReplaceAll(demoPath, "\\", "/")

	// Map name input
	cyan.Print("Map name (e.g. 'de_mirage'): ")
	mapName, _ = reader.ReadString('\n')
	mapName = strings.TrimSpace(mapName)

	// Summary
	fmt.Println()
	blue.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	white.Printf("  Demo: %s\n", demoPath)
	white.Printf("  Map:  %s\n", mapName)
	blue.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	// Start HTTP server
	contents, _ := os.ReadFile("./index.html")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(contents))
	})
	http.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(lastMapImg)
	})
	http.HandleFunc("/players", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(playersData)
	})
	http.HandleFunc("/miniradar", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
    <title>CS2 Radar - Mini</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            background: #0a0a0a;
            display: flex;
            align-items: center;
            justify-content: center;
            min-height: 100vh;
            overflow: hidden;
        }
        img {
            max-width: 100%;
            max-height: 100vh;
            object-fit: contain;
        }
    <\/style>
<\/head>
<body>
    <img id="radar" src="/map" />
    <script>
        setInterval(() => {
            document.getElementById('radar').src = '/map?' + new Date().getTime();
        }, 100);
    <\/script>
<\/body>
<\/html>`))
	})

	go http.ListenAndServe(":8080", nil)

	green.Println("✓ Server started at http://localhost:8080")
	yellow.Println("⏳ Waiting for demo file updates...")
	fmt.Println()

	fileInfo, err := os.Stat(demoPath)
	if err != nil {
		initialTime = time.Now()
	} else {
		initialTime = fileInfo.ModTime()
	}

	// Main loop
	for {
		fileInfo, err := os.Stat(demoPath)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if fileInfo.ModTime().After(initialTime) && fileInfo.Size() > 0 {
			initialTime = fileInfo.ModTime()
			f, err := os.Open(demoPath)
			if err != nil {
				red.Printf("✗ Failed to open: %s\n", err)
			} else {
				magenta.Printf("⟳ Processing [%s]\n", time.Now().Format("15:04:05"))
				processDemo(f)
				f.Close()
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func printHeader() {
	cyan.Println("╔════════════════════════════════════════════════╗")
	cyan.Println("║                                                ║")
	cyan.Println("║         CS2 REAL-TIME RADAR VISUALIZER         ║")
	cyan.Println("║                                                ║")
	cyan.Println("╚════════════════════════════════════════════════╝")
	fmt.Println()
	yellow.Println("Fork: https://github.com/2xxn/cs2-realtime-demo-radar-visualizer")
	red.Println("⚠ Warning: Use at your own risk")
	fmt.Println()
}

func detectCS2Path() string {
	// Try to get Steam path from Windows registry
	steamPath := getSteamPathFromRegistry()
	if steamPath != "" {
		// Check if CS2 is installed in the main Steam library
		cs2Path := filepath.Join(steamPath, "steamapps", "common", "Counter-Strike Global Offensive", "game", "csgo")
		if _, err := os.Stat(cs2Path); err == nil {
			return cs2Path
		}

		// Try to find additional library folders
		libraryFoldersPath := filepath.Join(steamPath, "steamapps", "libraryfolders.vdf")
		if additionalPaths := parseLibraryFolders(libraryFoldersPath); len(additionalPaths) > 0 {
			for _, libPath := range additionalPaths {
				cs2Path := filepath.Join(libPath, "steamapps", "common", "Counter-Strike Global Offensive", "game", "csgo")
				if _, err := os.Stat(cs2Path); err == nil {
					return cs2Path
				}
			}
		}
	}

	// Fallback: try common paths
	possiblePaths := []string{
		"C:/Program Files (x86)/Steam/steamapps/common/Counter-Strike Global Offensive/game/csgo",
		"D:/Steam/steamapps/common/Counter-Strike Global Offensive/game/csgo",
		"E:/Steam/steamapps/common/Counter-Strike Global Offensive/game/csgo",
		"C:/SteamLibrary/steamapps/common/Counter-Strike Global Offensive/game/csgo",
		"D:/SteamLibrary/steamapps/common/Counter-Strike Global Offensive/game/csgo",
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		possiblePaths = append(possiblePaths,
			filepath.Join(homeDir, ".steam/steam/steamapps/common/Counter-Strike Global Offensive/game/csgo"),
			filepath.Join(homeDir, ".local/share/Steam/steamapps/common/Counter-Strike Global Offensive/game/csgo"),
		)
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

func getSteamPathFromRegistry() string {
	// Try to open Steam registry key (64-bit)
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err != nil {
		// Try 32-bit registry
		k, err = registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, registry.QUERY_VALUE)
		if err != nil {
			return ""
		}
	}
	defer k.Close()

	// Read SteamPath value
	steamPath, _, err := k.GetStringValue("SteamPath")
	if err != nil {
		return ""
	}

	// Normalize path separators
	steamPath = strings.ReplaceAll(steamPath, "\\", "/")
	return steamPath
}

func parseLibraryFolders(vdfPath string) []string {
	var paths []string

	content, err := os.ReadFile(vdfPath)
	if err != nil {
		return paths
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "\"path\"") {
			// Extract path value
			parts := strings.Split(line, "\"")
			if len(parts) >= 4 {
				path := parts[3]
				path = strings.ReplaceAll(path, "\\\\", "/")
				path = strings.ReplaceAll(path, "\\", "/")
				paths = append(paths, path)
			}
		}
	}

	return paths
}

func processDemo(f *os.File) {
	defer func() {
		if r := recover(); r != nil {
			red.Printf("✗ Parsing error: %v\n", r)
		}
	}()

	p := demoinfocs.NewParser(f)

	var (
		mapMetadata ex.Map      = ex.GetMapMetadata(mapName)
		mapRadarImg image.Image = ex.GetMapRadar(mapName)
	)

	// Parse File to last tick to get latest positions
	err := p.ParseToEnd()
	if err != nil {
		// Silently ignore parsing errors during recording
	}

	// Check if map radar image was loaded
	if mapRadarImg == nil {
		return
	}

	// Create context for drawing
	dc := gg.NewContextForImage(mapRadarImg)

	// Clear previous player data
	playersData = []PlayerData{}

	// Get bomb position
	bomb := p.GameState().Bomb()
	var bombPos *r3.Vector
	if bomb != nil && bomb.Carrier == nil {
		// Bomb is dropped
		pos := bomb.Position()
		bombPos = &pos
	}

	// Get dropped weapons on ground
	type DroppedItem struct {
		Pos    r3.Vector
		Name   string
		IsKit  bool
		IsGood bool // High-value weapon
	}
	droppedItems := []DroppedItem{}

	// Get all weapons in the game
	for _, weapon := range p.GameState().Weapons() {
		if weapon == nil || weapon.Owner != nil {
			continue // Skip weapons that have owners
		}

		weaponType := weapon.Type.String()
		if weaponType == "EqBomb" || strings.Contains(weaponType, "C4") {
			continue // Skip bomb (already handled)
		}

		weaponName := strings.ReplaceAll(weaponType, "EqWeapon", "")
		weaponName = strings.ReplaceAll(weaponName, "Eq", "")

		// Check if it's a defuse kit
		isKit := strings.Contains(strings.ToLower(weaponType), "defusekit") || strings.Contains(strings.ToLower(weaponType), "defuser")

		// Check if it's a high-value weapon (rifles, snipers)
		isGood := false
		class := weapon.Class()
		if class == 3 || class == 4 || class == 5 { // SMG, Rifle, Heavy
			isGood = true
		}

		// Try to get position from entity
		if weapon.Entity != nil {
			pos := weapon.Entity.Position()
			if pos.X != 0 && pos.Y != 0 { // Valid position
				droppedItems = append(droppedItems, DroppedItem{
					Pos:    pos,
					Name:   weaponName,
					IsKit:  isKit,
					IsGood: isGood,
				})
			}
		}
	}

	for _, player := range p.GameState().Participants().Playing() {
		pos := player.Position()
		x, y := mapMetadata.TranslateScale(pos.X, pos.Y)

		// Get player info
		hp := player.Health()
		armor := player.Armor()
		money := player.Money()
		kills := player.Kills()
		deaths := player.Deaths()
		assists := player.Assists()
		name := player.Name
		isAlive := player.IsAlive()

		// Get weapons
		primaryWeapon := ""
		secondaryWeapon := ""
		hasBomb := false

		for _, weapon := range player.Weapons() {
			if weapon == nil {
				continue
			}

			weaponType := weapon.Type.String()
			weaponName := strings.ReplaceAll(weaponType, "EqWeapon", "")
			weaponName = strings.ReplaceAll(weaponName, "Eq", "")

			// Check if it's the bomb
			if weaponType == "EqBomb" || strings.Contains(weaponType, "C4") {
				hasBomb = true
				continue
			}

			// Categorize weapon
			class := weapon.Class()
			switch class {
			case 1, 2: // Pistols
				if secondaryWeapon == "" {
					secondaryWeapon = weaponName
				}
			case 3, 4, 5: // SMGs, Rifles, Heavy
				if primaryWeapon == "" {
					primaryWeapon = weaponName
				}
			}
		}

		// Add to players data for JSON API
		playersData = append(playersData, PlayerData{
			Name:            name,
			Health:          hp,
			Armor:           armor,
			Money:           money,
			Kills:           kills,
			Deaths:          deaths,
			Assists:         assists,
			Team:            int(player.Team),
			PrimaryWeapon:   primaryWeapon,
			SecondaryWeapon: secondaryWeapon,
			IsAlive:         isAlive,
			HasKit:          player.HasDefuseKit(),
			HasBomb:         hasBomb,
			IsPlanting:      player.IsPlanting,
			IsDefusing:      player.IsDefusing,
			IsScoped:        player.IsScoped(),
			UserID:          player.UserID, // For stable sorting
		})

		if !isAlive {
			continue // Skip drawing dead players
		}

		// Determine color based on team
		var col imgcolor.RGBA
		switch player.Team {
		case 2: // T
			col = imgcolor.RGBA{255, 80, 80, 255}
		case 3: // CT
			col = imgcolor.RGBA{80, 150, 255, 255}
		}

		// Draw player dot with outline
		dc.SetRGBA(0, 0, 0, 0.8)
		dc.DrawCircle(x, y, DOT_SIZE/2+1)
		dc.Fill()

		dc.SetRGBA(float64(col.R)/255, float64(col.G)/255, float64(col.B)/255, 1)
		dc.DrawCircle(x, y, DOT_SIZE/2)
		dc.Fill()

		// Draw HP text above player dot
		dc.SetRGBA(1, 1, 1, 1)
		dc.LoadFontFace("C:/Windows/Fonts/arialbd.ttf", 11)
		hpText := fmt.Sprintf("%d", hp)

		// HP color coding
		if hp > 75 {
			dc.SetRGBA(0.3, 1, 0.3, 1)
		} else if hp > 40 {
			dc.SetRGBA(1, 1, 0.3, 1)
		} else {
			dc.SetRGBA(1, 0.3, 0.3, 1)
		}

		hpTextWidth, _ := dc.MeasureString(hpText)

		// Draw text shadow
		dc.SetRGBA(0, 0, 0, 0.9)
		dc.DrawString(hpText, x-hpTextWidth/2+1, y-DOT_SIZE-3+1)

		// Draw HP text
		if hp > 75 {
			dc.SetRGBA(0.3, 1, 0.3, 1)
		} else if hp > 40 {
			dc.SetRGBA(1, 1, 0.3, 1)
		} else {
			dc.SetRGBA(1, 0.3, 0.3, 1)
		}
		dc.DrawString(hpText, x-hpTextWidth/2, y-DOT_SIZE-3)
	}

	// Draw dropped bomb
	if bombPos != nil {
		bx, by := mapMetadata.TranslateScale(bombPos.X, bombPos.Y)

		// Draw pulsing red circle for bomb
		dc.SetRGBA(1, 0, 0, 0.7)
		dc.DrawCircle(bx, by, 12)
		dc.Fill()

		dc.SetRGBA(1, 0.3, 0.3, 1)
		dc.DrawCircle(bx, by, 10)
		dc.Fill()

		// Draw bomb icon/text
		dc.SetRGBA(1, 1, 1, 1)
		dc.LoadFontFace("C:/Windows/Fonts/arialbd.ttf", 14)
		bombText := "💣"
		bombWidth, _ := dc.MeasureString(bombText)
		dc.DrawString(bombText, bx-bombWidth/2, by+5)
	}

	// Draw dropped items (weapons and kits)
	for _, item := range droppedItems {
		ix, iy := mapMetadata.TranslateScale(item.Pos.X, item.Pos.Y)

		if item.IsKit {
			// Draw defuse kit (green)
			dc.SetRGBA(0, 0.8, 0, 0.6)
			dc.DrawCircle(ix, iy, 6)
			dc.Fill()

			dc.SetRGBA(0.3, 1, 0.3, 1)
			dc.DrawCircle(ix, iy, 5)
			dc.Fill()
		} else if item.IsGood {
			// Draw high-value weapon (orange/yellow)
			dc.SetRGBA(1, 0.6, 0, 0.6)
			dc.DrawRectangle(ix-5, iy-2, 10, 4)
			dc.Fill()

			dc.SetRGBA(1, 0.8, 0.2, 1)
			dc.DrawRectangle(ix-4, iy-1.5, 8, 3)
			dc.Fill()
		} else {
			// Draw regular weapon (gray)
			dc.SetRGBA(0.5, 0.5, 0.5, 0.5)
			dc.DrawRectangle(ix-4, iy-1.5, 8, 3)
			dc.Fill()
		}

		// Draw weapon name below item (small text)
		if item.IsKit {
			dc.SetRGBA(0.3, 1, 0.3, 1)
			dc.LoadFontFace("C:/Windows/Fonts/arial.ttf", 7)
			kitText := "KIT"
			kitWidth, _ := dc.MeasureString(kitText)
			dc.DrawString(kitText, ix-kitWidth/2, iy+10)
		} else if item.IsGood && len(item.Name) > 0 {
			dc.SetRGBA(1, 0.8, 0.2, 1)
			dc.LoadFontFace("C:/Windows/Fonts/arial.ttf", 7)
			nameWidth, _ := dc.MeasureString(item.Name)
			dc.DrawString(item.Name, ix-nameWidth/2, iy+10)
		}
	}

	// Sort players by Team and UserID for stable order
	sort.SliceStable(playersData, func(i, j int) bool {
		if playersData[i].Team != playersData[j].Team {
			return playersData[i].Team < playersData[j].Team
		}
		return playersData[i].UserID < playersData[j].UserID
	})

	buffer := bytes.NewBuffer(nil)
	png.Encode(buffer, dc.Image())
	lastMapImg = buffer.Bytes()
}
