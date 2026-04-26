package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/1051093860/gamedepot/internal/app"
	"github.com/1051093860/gamedepot/internal/workspace"
)

func Classify(ctx context.Context, start string, targetPath string, jsonOut bool, includeUnmatched bool) error {
	a, err := app.Load(ctx, start)
	if err != nil {
		return err
	}

	targetPath = filepath.ToSlash(targetPath)
	if targetPath != "" {
		clean, err := workspace.CleanRelPath(targetPath)
		if err != nil {
			return err
		}
		targetPath = clean
		targetAbs, err := workspace.SafeJoin(a.Root, targetPath)
		if err != nil {
			return err
		}
		if _, err := os.Stat(targetAbs); os.IsNotExist(err) {
			class, err := workspace.ClassifyRel(targetPath, a.Config)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(class)
			}
			printClassification(class)
			return nil
		} else if err != nil {
			return err
		}
	}

	items, err := workspace.ClassifyWalk(a.Root, a.Config, targetPath, includeUnmatched)
	if err != nil {
		return err
	}

	if jsonOut {
		return printJSON(items)
	}

	if len(items) == 0 {
		fmt.Println("No classified files")
		return nil
	}

	fmt.Println("mode    path                                      kind              rule")
	for _, item := range items {
		printClassification(item)
	}

	return nil
}

func printClassification(item workspace.Classification) {
	mode := string(item.Mode)
	if item.IsDir {
		mode += "/"
	}
	fmt.Printf("%-7s %-40s  %-16s  %s\n", mode, item.Path, item.Kind, item.RulePattern)
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
