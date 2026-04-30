package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	devDirName             = ".browseros-dev"
	limaVMName             = "browseros-vm"
	openClawImage          = "ghcr.io/openclaw/openclaw:2026.4.12"
	openClawContainerName  = "browseros-openclaw-openclaw-gateway-1"
	openClawSetupContainer = openClawContainerName + "-setup"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Guide destructive BrowserOS dev profile and VM resets",
	Long:  "Walks through safe cleanup, VM shutdown/deletion, OpenClaw container/image removal, and full ~/.browseros-dev reset.",
	RunE:  runReset,
}

type devPaths struct {
	Root     string
	LimaHome string
}

type resetPrompt struct {
	Title  string
	Body   string
	Action string
}

type limaListEntry struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type podmanMachineEntry struct {
	Name    string `json:"Name"`
	Running bool   `json:"Running"`
}

func init() {
	rootCmd.AddCommand(resetCmd)
}

// runReset walks developers through escalating reset options without hiding the blast radius.
func runReset(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	reader := bufio.NewReader(os.Stdin)
	paths, err := resolveDevPaths()
	if err != nil {
		return err
	}

	printResetOverview(out, paths)

	if ok, err := confirmYesNo(out, reader, resetPrompt{
		Title:  "Run safe cleanup first?",
		Body:   "This stops old dev watch processes, clears dev ports, and removes temporary /tmp browser profiles. It does not touch saved dev data.",
		Action: "Run safe cleanup",
	}); err != nil {
		return err
	} else if ok {
		if err := runSafeCleanup(out, safeCleanupOptions{ports: true, temps: true}); err != nil {
			return err
		}
	}

	limactlPath, err := exec.LookPath("limactl")
	if err != nil {
		fmt.Fprintf(out, "%s Lima CLI not found; VM and OpenClaw reset steps are unavailable. Install with %s.\n", warnStyle.Sprint("Skipping:"), commandStyle.Sprint("brew install lima"))
		if err := maybeResetLegacyPodman(out, reader); err != nil {
			return err
		}
		return maybeDeleteDevProfile(out, reader, paths)
	}

	vm, err := findVM(limactlPath, paths.LimaHome)
	if err != nil {
		fmt.Fprintf(out, "%s could not inspect Lima VMs: %v\n", warnStyle.Sprint("Warning:"), err)
		if err := maybeResetLegacyPodman(out, reader); err != nil {
			return err
		}
		return maybeDeleteDevProfile(out, reader, paths)
	}
	if vm == nil {
		fmt.Fprintf(out, "%s %s was not found in %s.\n", dimStyle.Sprint("Not found:"), limaVMName, pathStyle.Sprint(paths.LimaHome))
		if err := maybeResetLegacyPodman(out, reader); err != nil {
			return err
		}
		return maybeDeleteDevProfile(out, reader, paths)
	}

	fmt.Fprintf(out, "%s %s %s\n", labelStyle.Sprint("Found VM:"), commandStyle.Sprint(vm.Name), dimStyle.Sprintf("(%s)", vm.Status))
	if strings.EqualFold(vm.Status, "Running") {
		if err := maybeResetOpenClaw(out, reader, limactlPath, paths.LimaHome); err != nil {
			return err
		}
		if ok, err := confirmYesNo(out, reader, resetPrompt{
			Title:  "Stop VM?",
			Body:   "This shuts down browseros-vm. The VM, containers, images, and profile data stay on disk.",
			Action: "Stop browseros-vm",
		}); err != nil {
			return err
		} else if ok {
			if err := runLimactl(out, limactlPath, paths.LimaHome, "stop", limaVMName); err != nil {
				return err
			}
			fmt.Fprintln(out, successStyle.Sprint("VM stopped."))
			vm.Status = "Stopped"
		}
	} else {
		fmt.Fprintln(out, dimStyle.Sprint("OpenClaw container/image reset needs the VM running; skipping those steps."))
	}

	if ok, err := confirmYesNo(out, reader, resetPrompt{
		Title:  "Delete VM?",
		Body:   "This deletes the Lima VM and its container store. ~/.browseros-dev remains. OpenClaw will be pulled again next time.",
		Action: "Delete browseros-vm",
	}); err != nil {
		return err
	} else if ok {
		if err := runLimactl(out, limactlPath, paths.LimaHome, "delete", "--force", limaVMName); err != nil {
			return err
		}
		fmt.Fprintln(out, successStyle.Sprint("VM deleted."))
	}

	if err := maybeResetLegacyPodman(out, reader); err != nil {
		return err
	}

	return maybeDeleteDevProfile(out, reader, paths)
}

func resolveDevPaths() (devPaths, error) {
	if override := strings.TrimSpace(os.Getenv("BROWSEROS_DIR")); override != "" {
		root, err := filepath.Abs(override)
		if err != nil {
			return devPaths{}, err
		}
		return devPaths{Root: root, LimaHome: filepath.Join(root, "lima")}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return devPaths{}, err
	}
	root := filepath.Join(home, devDirName)
	return devPaths{Root: root, LimaHome: filepath.Join(root, "lima")}, nil
}

func printResetOverview(out io.Writer, paths devPaths) {
	fmt.Fprintln(out, headerStyle.Sprint("BrowserOS dev reset"))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "This can reset parts of %s. Pick the smallest reset that matches the problem.\n", pathStyle.Sprint(paths.Root))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s %s\n", labelStyle.Sprint("Stop VM:"), dimStyle.Sprint("Shuts down browseros-vm. Keeps data."))
	fmt.Fprintf(out, "  %s %s\n", labelStyle.Sprint("Delete VM:"), dimStyle.Sprint("Removes Lima/container state. Keeps the dev profile."))
	fmt.Fprintf(out, "  %s %s\n", labelStyle.Sprint("Remove OpenClaw container:"), dimStyle.Sprint("Keeps the downloaded OpenClaw image."))
	fmt.Fprintf(out, "  %s %s\n", labelStyle.Sprint("Remove OpenClaw image:"), dimStyle.Sprint("Next startup pulls it again."))
	fmt.Fprintf(out, "  %s %s\n", warnStyle.Sprint("Delete dev profile:"), dimStyle.Sprint("Deletes the dev profile root and dev-local BrowserOS data."))
	fmt.Fprintln(out)
}

func confirmYesNo(out io.Writer, r *bufio.Reader, prompt resetPrompt) (bool, error) {
	fmt.Fprintln(out, labelStyle.Sprint(prompt.Title))
	fmt.Fprintln(out, prompt.Body)
	if prompt.Action != "" {
		fmt.Fprintf(out, "%s %s\n", labelStyle.Sprint("Action:"), commandStyle.Sprint(prompt.Action))
	}
	fmt.Fprint(out, labelStyle.Sprint("Continue?")+" [y/N]: ")
	line, err := r.ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	line = strings.TrimSpace(strings.ToLower(line))
	fmt.Fprintln(out)
	return line == "y" || line == "yes", nil
}

func confirmTyped(out io.Writer, r *bufio.Reader, title string, body string, token string) (bool, error) {
	fmt.Fprintln(out, warnStyle.Sprint(title))
	fmt.Fprintln(out, body)
	for {
		fmt.Fprintf(out, "%s %s %s: ", labelStyle.Sprint("Type"), commandStyle.Sprint(token), labelStyle.Sprint("to continue"))
		line, err := r.ReadString('\n')
		if err != nil && len(line) == 0 {
			return false, err
		}
		if strings.TrimSpace(line) == token {
			fmt.Fprintln(out)
			return true, nil
		}
		if strings.TrimSpace(line) == "" {
			fmt.Fprintln(out)
			return false, nil
		}
		fmt.Fprintln(out, warnStyle.Sprint("Confirmation did not match. Press Enter to skip or try again."))
	}
}

func maybeResetOpenClaw(out io.Writer, reader *bufio.Reader, limactlPath string, limaHome string) error {
	if ok, err := confirmYesNo(out, reader, resetPrompt{
		Title:  "Remove OpenClaw container?",
		Body:   "This removes the current gateway/setup containers. The downloaded OpenClaw image stays in the VM.",
		Action: "nerdctl rm -f " + openClawContainerName + " " + openClawSetupContainer,
	}); err != nil {
		return err
	} else if ok {
		script := fmt.Sprintf(
			"nerdctl rm -f %s %s >/dev/null 2>&1 || true",
			openClawContainerName,
			openClawSetupContainer,
		)
		if err := runInVM(out, limactlPath, limaHome, "sh", "-lc", script); err != nil {
			return err
		}
		fmt.Fprintln(out, successStyle.Sprint("OpenClaw containers removed if present."))
	}

	if ok, err := confirmYesNo(out, reader, resetPrompt{
		Title:  "Remove OpenClaw image?",
		Body:   "This deletes ghcr.io/openclaw/openclaw:2026.4.12 from the VM. Next startup pulls it again.",
		Action: "nerdctl image rm " + openClawImage,
	}); err != nil {
		return err
	} else if ok {
		script := fmt.Sprintf("nerdctl image rm %s >/dev/null 2>&1 || true", openClawImage)
		if err := runInVM(out, limactlPath, limaHome, "sh", "-lc", script); err != nil {
			return err
		}
		fmt.Fprintln(out, successStyle.Sprint("OpenClaw image removed if present."))
	}
	return nil
}

func maybeDeleteDevProfile(out io.Writer, reader *bufio.Reader, paths devPaths) error {
	ok, err := confirmTyped(
		out,
		reader,
		"Delete dev profile?",
		fmt.Sprintf("This deletes %s. It removes BrowserOS dev data plus VM/OpenClaw state.", pathStyle.Sprint(paths.Root)),
		"DELETE",
	)
	if err != nil || !ok {
		return err
	}
	if err := validateDevProfileRootForDeletion(paths.Root); err != nil {
		return err
	}
	if err := os.RemoveAll(paths.Root); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s %s\n", successStyle.Sprint("Deleted:"), pathStyle.Sprint(paths.Root))
	return nil
}

func maybeResetLegacyPodman(out io.Writer, reader *bufio.Reader) error {
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		return nil
	}
	machines, err := listPodmanMachines(podmanPath)
	if err != nil {
		fmt.Fprintf(out, "%s could not inspect legacy Podman machines: %v\n", warnStyle.Sprint("Warning:"), err)
		return nil
	}
	if len(machines) == 0 {
		return nil
	}

	fmt.Fprintln(out, headerStyle.Sprint("Legacy Podman VM cleanup"))
	fmt.Fprintln(out, "BrowserOS used Podman before the Lima VM runtime. These machines are legacy for this dev flow.")
	for _, machine := range machines {
		state := "Stopped"
		if machine.Running {
			state = "Running"
		}
		fmt.Fprintf(out, "  %s %s\n", commandStyle.Sprint(machine.Name), dimStyle.Sprintf("(%s)", state))
	}
	fmt.Fprintln(out, dimStyle.Sprint("Future reset flows can add more legacy cleanup checks here."))
	fmt.Fprintln(out)

	for i := range machines {
		machine := machines[i]
		if machine.Running {
			if ok, err := confirmYesNo(out, reader, resetPrompt{
				Title:  "Stop legacy Podman machine?",
				Body:   fmt.Sprintf("This stops legacy Podman machine %s. It does not delete the machine.", machine.Name),
				Action: "podman machine stop " + machine.Name,
			}); err != nil {
				return err
			} else if ok {
				if err := runCommand(out, podmanPath, "machine", "stop", machine.Name); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s %s\n", successStyle.Sprint("Stopped:"), commandStyle.Sprint(machine.Name))
				machines[i].Running = false
			}
		}

		if ok, err := confirmYesNo(out, reader, resetPrompt{
			Title:  "Delete legacy Podman machine?",
			Body:   fmt.Sprintf("This deletes legacy Podman machine %s. Use this when cleaning up the old VM runtime.", machine.Name),
			Action: "podman machine rm --force " + machine.Name,
		}); err != nil {
			return err
		} else if ok {
			if err := runCommand(out, podmanPath, "machine", "rm", "--force", machine.Name); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s %s\n", successStyle.Sprint("Deleted:"), commandStyle.Sprint(machine.Name))
		}
	}
	return nil
}

func listPodmanMachines(podmanPath string) ([]podmanMachineEntry, error) {
	cmd := exec.Command(podmanPath, "machine", "ls", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parsePodmanMachineList(output)
}

func parsePodmanMachineList(output []byte) ([]podmanMachineEntry, error) {
	if strings.TrimSpace(string(output)) == "" {
		return nil, nil
	}
	var machines []podmanMachineEntry
	if err := json.Unmarshal(output, &machines); err != nil {
		return nil, err
	}
	return machines, nil
}

func validateDevProfileRootForDeletion(root string) error {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if cleanRoot == string(filepath.Separator) {
		return fmt.Errorf("refusing to delete filesystem root")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cleanHome, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	if cleanRoot == cleanHome {
		return fmt.Errorf("refusing to delete home directory %s", cleanRoot)
	}
	if !isPathInside(cleanRoot, cleanHome) {
		return fmt.Errorf("refusing to delete path outside home directory: %s", cleanRoot)
	}
	return nil
}

func isPathInside(path string, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != "" && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func findVM(limactlPath string, limaHome string) (*limaListEntry, error) {
	cmd := limactlCommand(limactlPath, limaHome, "list", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	entries, err := parseLimaListOutput(output)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Name == limaVMName {
			return &entries[i], nil
		}
	}
	return nil, nil
}

func parseLimaListOutput(output []byte) ([]limaListEntry, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, nil
	}

	var entries []limaListEntry
	if err := json.Unmarshal([]byte(trimmed), &entries); err == nil {
		return entries, nil
	}

	var single limaListEntry
	if err := json.Unmarshal([]byte(trimmed), &single); err == nil {
		return []limaListEntry{single}, nil
	}

	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry limaListEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func runLimactl(out io.Writer, limactlPath string, limaHome string, args ...string) error {
	cmd := limactlCommand(limactlPath, limaHome, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

func runInVM(out io.Writer, limactlPath string, limaHome string, args ...string) error {
	shellArgs := limactlShellArgs(args...)
	return runLimactl(out, limactlPath, limaHome, shellArgs...)
}

func limactlShellArgs(args ...string) []string {
	return append([]string{"shell", "--workdir", "/", limaVMName, "--"}, args...)
}

func limactlCommand(limactlPath string, limaHome string, args ...string) *exec.Cmd {
	cmd := exec.Command(limactlPath, args...)
	cmd.Env = append(os.Environ(), "LIMA_HOME="+limaHome)
	return cmd
}

func runCommand(out io.Writer, path string, args ...string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}
