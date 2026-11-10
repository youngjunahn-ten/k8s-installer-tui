package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ten1010-io/k8s-installer-tui/internal/fileio"
	"github.com/ten1010-io/k8s-installer-tui/internal/schema"
	"github.com/ten1010-io/k8s-installer-tui/internal/state"
	"github.com/ten1010-io/k8s-installer-tui/internal/ui"
)

var version = "dev"

func main() {
	// termenv (lipgloss) detects color support from COLORTERM/TERM env vars.
	// SSH sessions often don't forward COLORTERM, causing lipgloss to fall back
	// to 16-color mode even on 256-color terminals. Force truecolor so lipgloss
	// always outputs 256-color escape codes, matching what nmtui/k9s see.
	if os.Getenv("COLORTERM") == "" {
		os.Setenv("COLORTERM", "truecolor")
	}
	inventoryPath := flag.String("inventory", "inventory.yml", "inventory.yml 경로")
	varsPath := flag.String("vars", "group_vars/all/vars.yml", "vars.yml 경로")
	schemaPath := flag.String("schema", "", "vars 스키마 경로 (기본: inventory.yml 옆의 schema/vars.schema.json)")
	showVersion := flag.Bool("version", false, "버전 출력")
	flag.Parse()

	if *showVersion {
		fmt.Println("k8s-installer-tui", version)
		os.Exit(0)
	}

	// Resolve absolute paths
	invAbs, err := filepath.Abs(*inventoryPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "경로 오류: %v\n", err)
		os.Exit(1)
	}
	varsAbs, err := filepath.Abs(*varsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "경로 오류: %v\n", err)
		os.Exit(1)
	}

	// Start from defaults, then overlay existing files
	s := state.DefaultState()

	if err := fileio.LoadInventory(invAbs, s); err != nil {
		fmt.Fprintf(os.Stderr, "inventory.yml 로드 오류: %v\n", err)
		os.Exit(1)
	}
	if err := fileio.LoadVars(varsAbs, s); err != nil {
		fmt.Fprintf(os.Stderr, "vars.yml 로드 오류: %v\n", err)
		os.Exit(1)
	}

	// An installer that says what its vars.yml accepts ships the schema beside
	// inventory.yml. With it the vars screen is drawn from the schema, so it
	// matches that installer; without it the screens are the ones written for
	// the installer before the split.
	schemaFile := *schemaPath
	if schemaFile == "" {
		schemaFile = filepath.Join(filepath.Dir(invAbs), "schema", "vars.schema.json")
	}
	var app *ui.App
	if _, err := os.Stat(schemaFile); err == nil {
		sch, err := schema.Load(schemaFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "스키마 로드 오류 (%s): %v\n", schemaFile, err)
			os.Exit(1)
		}
		values, err := fileio.LoadVarsValues(varsAbs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vars.yml 로드 오류: %v\n", err)
			os.Exit(1)
		}
		sch.Fill(s, values)
		app = ui.NewSchemaApp(s, invAbs, varsAbs, version, sch)
	} else if *schemaPath != "" {
		fmt.Fprintf(os.Stderr, "스키마 파일이 없습니다: %s\n", schemaFile)
		os.Exit(1)
	} else {
		app = ui.NewApp(s, invAbs, varsAbs, version)
	}
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		os.Exit(1)
	}
}
