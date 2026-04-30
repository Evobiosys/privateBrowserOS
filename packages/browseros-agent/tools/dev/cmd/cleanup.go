package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

	"browseros-dev/proc"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Kill port processes and remove orphaned temp directories",
	Long:  "Stops old dev watch processes, clears dev/test ports, and removes orphaned browseros-* temp directories.",
	RunE:  runCleanup,
}

var (
	cleanupPorts bool
	cleanupTemps bool
	cleanupQuick bool
	cleanupYes   bool
)

type safeCleanupOptions struct {
	ports bool
	temps bool
}

func init() {
	cleanupCmd.Flags().BoolVar(&cleanupPorts, "ports", false, "Only kill port processes")
	cleanupCmd.Flags().BoolVar(&cleanupTemps, "temps", false, "Only remove temp directories")
	cleanupCmd.Flags().BoolVar(&cleanupQuick, "quick", false, "Run safe cleanup only")
	cleanupCmd.Flags().BoolVar(&cleanupYes, "yes", false, "Answer yes to the safe cleanup prompt")
	rootCmd.AddCommand(cleanupCmd)
}

// runCleanup performs the non-destructive daily cleanup path for local dev.
func runCleanup(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	if !cleanupYes && !cleanupQuick {
		ok, err := confirmYesNo(out, bufio.NewReader(os.Stdin), resetPrompt{
			Title:  "Run safe cleanup?",
			Body:   "Stops old dev watch processes, clears dev ports, and removes temporary /tmp browser profiles. This does not touch ~/.browseros-dev, Lima, containers, images, or saved dev data.",
			Action: "Run safe cleanup",
		})
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, dimStyle.Sprint("Skipped."))
			return nil
		}
	}
	return runSafeCleanup(out, safeCleanupOptions{
		ports: !cleanupTemps || cleanupPorts,
		temps: !cleanupPorts || cleanupTemps,
	})
}

// runSafeCleanup is shared by cleanup and reset before any destructive repair steps.
func runSafeCleanup(out io.Writer, opts safeCleanupOptions) error {
	if opts.ports {
		ports := proc.DefaultLocalPorts()
		stopped, err := proc.StopAllWatchProcesses(3 * time.Second)
		if err != nil {
			return err
		}
		if stopped > 0 {
			fmt.Fprintf(out, "%s stopped %d old dev watch process group(s)\n", successStyle.Sprint("Stopped:"), stopped)
		}
		killedBrowsers, err := proc.KillBrowserProcessesForDevProfiles(3 * time.Second)
		if err != nil {
			return err
		}
		if killedBrowsers > 0 {
			fmt.Fprintf(out, "%s stopped %d BrowserOS dev/test profile process(es)\n", successStyle.Sprint("Stopped:"), killedBrowsers)
		}
		fmt.Fprintf(out, "%s ports %d, %d, %d\n", labelStyle.Sprint("Clearing:"), ports.CDP, ports.Server, ports.Extension)
		if err := proc.KillPortsAndWait(ports, 3*time.Second); err != nil {
			return err
		}
		fmt.Fprintln(out, successStyle.Sprint("Ports cleared."))
	}

	if opts.temps {
		n := proc.CleanupTempDirs("browseros-test-", "browseros-dev-")
		if n > 0 {
			fmt.Fprintf(out, "%s removed %d temp directories\n", successStyle.Sprint("Removed:"), n)
		} else {
			fmt.Fprintln(out, dimStyle.Sprint("No orphaned temp directories found."))
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, successStyle.Sprint("Cleanup complete."))
	return nil
}
