package main

import (
	"context"
	"fmt"
	"time"

	"mctrl/internal/config"
	"mctrl/internal/tmux"
	"mctrl/internal/work"
)

func runWork(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: mctrl work list | mctrl work archive --confirm <id>")
	}
	root, err := config.StateDir()
	if err != nil {
		return err
	}
	profile, err := config.ActiveProfile()
	if err != nil {
		return err
	}
	if err := config.ValidateRuntimeIdentity(root, profile.Name); err != nil {
		return err
	}
	store := work.NewStore(root)
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: mctrl work list")
		}
		items, err := store.List()
		if err != nil {
			return err
		}
		for _, item := range items {
			status := string(item.State)
			if item.ArchivedAt != nil {
				status = "ARCHIVED (exit unknown)"
				if item.Terminal() {
					status = "ARCHIVED (" + string(item.State) + ")"
				}
			} else if item.Active() && item.RecoveryStatus != "" {
				status = "NEEDS REVIEW"
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", item.ID, status, item.ProjectID, item.RecoveryStatus)
		}
		return nil
	case "archive":
		fs := flagSet("work archive")
		confirm := fs.Bool("confirm", false, "confirm archival; Session and processes must be absent")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if !*confirm || fs.NArg() != 1 {
			return fmt.Errorf("usage: mctrl work archive --confirm <id>")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		item, err := work.ArchiveMissing(ctx, store, tmux.New(), fs.Arg(0))
		if err != nil {
			return err
		}
		fmt.Printf("Archived %s. History is retained; no exit result was invented.\n", item.ID)
		return nil
	default:
		return fmt.Errorf("unknown work command %q", args[0])
	}
}
