package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	xdgDataDirsEnvKey = "XDG_DATA_DIRS"
	applicationsPath  = "applications"
	desktopSuffix     = ".desktop"
)

func main() {
	xdgDataDirsEnv, ok := os.LookupEnv(xdgDataDirsEnvKey)
	if !ok {
		fmt.Fprintf(os.Stderr, "$%s not set\n", xdgDataDirsEnvKey)
		os.Exit(1)
	}

	xdgDataDirs := strings.Split(xdgDataDirsEnv, ":")
	applications, err := find(xdgDataDirs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "find paths: %v\n", err)
		os.Exit(1)
	}

	sort.Slice(applications, func(i, j int) bool {
		return applications[i].path > applications[j].path
	})

	for _, appl := range applications {
		fmt.Fprintf(os.Stdout, "%s\t%s\n", appl.path, appl.command)
	}
}

type application struct {
	path    string
	command string
}

func find(xdgDataDirs []string) ([]*application, error) {
	applicationFiles := make(chan string)
	applications := make(chan *application)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			for applicationFile := range applicationFiles {
				appl, err := parse(applicationFile)
				if err != nil {
					log.Printf("error checking file %q: %v", applicationFile, err)
					continue
				}
				if appl != nil {
					applications <- appl
				}
			}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(applications)
	}()

	go func() {
		for _, dataDir := range xdgDataDirs {
			applicationDir := filepath.Join(dataDir, applicationsPath)
			dirEnt, err := os.ReadDir(applicationDir)
			if err != nil {
				continue
			}
			for _, ent := range dirEnt {
				if ent.IsDir() || !strings.HasSuffix(ent.Name(), desktopSuffix) {
					continue
				}
				applicationFiles <- filepath.Join(applicationDir, ent.Name())
			}
		}
		close(applicationFiles)
	}()

	var results []*application
	for appl := range applications {
		results = append(results, appl)
	}

	return results, nil
}

// we don't care about passing arguments
// https://specifications.freedesktop.org/desktop-entry-spec/latest/ar01s07.html
var commandArgReplacer = strings.NewReplacer(
	"%f", "", "%F", "", "%u", "", "%U", "",
	"%d", "", "%D", "", "%n", "", "%N", "",
	"%i", "", "%c", "", "%k", "", "%v", "",
	"%m", "", "@@u", "", "@@", "",
)

func parse(applicationFile string) (*application, error) {
	f, err := os.Open(applicationFile)
	if err != nil {
		return nil, fmt.Errorf("open application file: %w", err)
	}
	defer f.Close()

	var hasApplication bool
	var command string

	reader := bufio.NewScanner(f)
sc:
	for reader.Scan() {
		switch line := reader.Text(); {
		case strings.HasPrefix(line, "NoDisplay=true"):
			return nil, nil
		case strings.HasPrefix(line, "Terminal=true"):
			return nil, nil
		case strings.HasPrefix(line, "Type=Application"):
			hasApplication = true
		case strings.HasPrefix(line, "Exec="):
			_, command, _ = strings.Cut(line, "=")
		case strings.TrimSpace(line) == "":
			break sc // only read first block
		}
	}

	if !hasApplication || command == "" {
		return nil, nil
	}

	return &application{
		path:    applicationFile,
		command: commandArgReplacer.Replace(command),
	}, nil
}
