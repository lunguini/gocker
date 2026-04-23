package compose

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lunguini/gocker/engine"
)

// Orchestrator manages compose project lifecycle.
type Orchestrator struct {
	eng engine.Runtime
}

func NewOrchestrator(eng engine.Runtime) *Orchestrator {
	return &Orchestrator{eng: eng}
}

// UpOptions configures the up command.
type UpOptions struct {
	File    string
	Detach  bool
	Project string // override project name
}

// Up starts all services in dependency order.
func (o *Orchestrator) Up(ctx context.Context, opts UpOptions) error {
	cf, absFile, err := Load(opts.File)
	if err != nil {
		return err
	}

	project := opts.Project
	if project == "" {
		project = ProjectName(absFile)
	}

	order, err := DependencyOrder(cf)
	if err != nil {
		return err
	}

	// Load existing state or create new
	state, _ := LoadProject(project)
	if state == nil {
		state = &ProjectState{
			Name:     project,
			Dir:      absFile,
			File:     absFile,
			Services: make(map[string]ServiceState),
		}
	}

	// Create networks
	for netName, net := range cf.Networks {
		if net.External {
			continue
		}
		fullName := project + "_" + netName
		fmt.Printf("Creating network %s\n", fullName)
		if err := o.eng.NetworkCreate(ctx, fullName); err != nil {
			// Network may already exist
			if !strings.Contains(err.Error(), "already exists") {
				fmt.Fprintf(os.Stderr, "Warning: creating network %s: %v\n", fullName, err)
			}
		} else {
			state.Networks = appendUnique(state.Networks, fullName)
		}
	}

	// Create default network if any service uses no explicit networks
	needsDefault := false
	for _, svc := range cf.Services {
		if len(svc.Networks) == 0 {
			needsDefault = true
			break
		}
	}
	defaultNet := project + "_default"
	if needsDefault {
		fmt.Printf("Creating network %s\n", defaultNet)
		if err := o.eng.NetworkCreate(ctx, defaultNet); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				fmt.Fprintf(os.Stderr, "Warning: creating default network: %v\n", err)
			}
		} else {
			state.Networks = appendUnique(state.Networks, defaultNet)
		}
	}

	// Create volumes using Apple's container CLI (proper VM-native volumes
	// with correct ownership/permissions inside the VM).
	for volName, vol := range cf.Volumes {
		if vol.External {
			continue
		}
		fullName := project + "_" + volName
		fmt.Printf("Creating volume %s\n", fullName)
		if err := o.eng.VolumeCreate(ctx, fullName); err != nil {
			if !strings.Contains(err.Error(), "already exists") {
				fmt.Fprintf(os.Stderr, "Warning: creating volume %s: %v\n", fullName, err)
			}
		}
		state.Volumes = appendUnique(state.Volumes, fullName)
	}

	// Start services in dependency order
	for _, svcName := range order {
		svc := cf.Services[svcName]
		containerName := containerNameForService(project, svcName, svc)

		// Check if already running
		if existing, ok := state.Services[svcName]; ok {
			status := o.getContainerStatus(ctx, existing.ContainerID)
			if status == "running" {
				fmt.Printf("  %s is up to date\n", svcName)
				continue
			}
			if status == "stopped" {
				fmt.Printf("  Starting %s\n", svcName)
				if err := o.eng.ContainerStart(ctx, existing.ContainerID); err == nil {
					existing.Status = "running"
					state.Services[svcName] = existing
					continue
				}
			}
			// Container gone, remove stale reference
			_ = o.eng.ContainerRemove(ctx, existing.ContainerID, true)
		}

		// Pull image if needed
		if svc.Image != "" {
			fmt.Printf("  Pulling %s\n", svc.Image)
			if err := o.eng.ImagePull(ctx, svc.Image, engine.ImagePullOpts{}); err != nil {
				// Image might already exist locally
				fmt.Fprintf(os.Stderr, "  Warning: pulling %s: %v\n", svc.Image, err)
			}
		}

		// Build run args
		args := o.buildRunArgs(project, svcName, svc, cf, defaultNet)

		fmt.Printf("  Creating %s\n", svcName)

		// Clean up any orphaned container with same name
		_ = o.eng.ContainerRemove(ctx, containerName, true)

		if err := o.eng.ContainerRun(ctx, args, false); err != nil {
			_ = o.eng.ContainerRemove(ctx, containerName, true)
			return fmt.Errorf("starting service %q: %w", svcName, err)
		}

		state.Services[svcName] = ServiceState{
			Service:     svcName,
			ContainerID: containerName,
			Image:       svc.Image,
			Status:      "running",
		}

		fmt.Printf("  Started %s\n", svcName)
	}

	if err := SaveProject(state); err != nil {
		return fmt.Errorf("saving project state: %w", err)
	}

	fmt.Printf("\nProject %q is up with %d service(s)\n", project, len(order))
	return nil
}

func (o *Orchestrator) buildRunArgs(project, svcName string, svc Service, cf *ComposeFile, defaultNet string) []string {
	var args []string

	containerName := containerNameForService(project, svcName, svc)

	// Always detach for compose
	args = append(args, "-d")
	args = append(args, "--name", containerName)

	// Ports
	for _, p := range svc.Ports {
		args = append(args, "-p", p)
	}

	// Volumes — resolve named volumes to project-scoped names
	for _, v := range svc.Volumes {
		resolved := resolveVolume(project, v, cf)
		args = append(args, "-v", resolved)
	}

	// Apple's container CLI formats volumes with ext4, which includes a
	// lost+found directory. Some images (e.g. PostgreSQL) refuse to init
	// into a non-empty directory. Auto-inject env vars that redirect the
	// data directory to a subdirectory of the mount point.
	autoEnv := volumeDataDirEnv(svc)
	if svc.Environment == nil && len(autoEnv) > 0 {
		svc.Environment = make(Environment)
	}
	for k, v := range autoEnv {
		if _, exists := svc.Environment[k]; !exists {
			svc.Environment[k] = v
		}
	}

	// Environment
	envKeys := make([]string, 0, len(svc.Environment))
	for k := range svc.Environment {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		v := svc.Environment[k]
		if v == "" {
			// Inherit from host
			if hostVal, ok := os.LookupEnv(k); ok {
				args = append(args, "-e", k+"="+hostVal)
			}
		} else {
			args = append(args, "-e", k+"="+v)
		}
	}

	// Network
	if len(svc.Networks) > 0 {
		// Use first network
		netName := project + "_" + string(svc.Networks[0])
		args = append(args, "--network", netName)
	} else {
		args = append(args, "--network", defaultNet)
	}

	// Working dir
	if svc.WorkingDir != "" {
		args = append(args, "-w", svc.WorkingDir)
	}

	// Memory
	if svc.Memory != "" {
		args = append(args, "-m", svc.Memory)
	}

	// Image
	args = append(args, svc.Image)

	// Command
	if len(svc.Command) > 0 {
		args = append(args, svc.Command...)
	}

	return args
}

// DownOptions configures the down command.
type DownOptions struct {
	File    string
	Project string
	Volumes bool // also remove volumes
}

// Down stops and removes all services, then cleans up networks/volumes.
func (o *Orchestrator) Down(ctx context.Context, opts DownOptions) error {
	cf, absFile, err := Load(opts.File)
	if err != nil {
		return err
	}

	project := opts.Project
	if project == "" {
		project = ProjectName(absFile)
	}

	state, err := LoadProject(project)
	if err != nil {
		// No state — try to stop based on naming convention
		fmt.Printf("No state found for project %q, cleaning up by name...\n", project)
		state = &ProjectState{
			Name:     project,
			Services: make(map[string]ServiceState),
		}
		// Build service list from compose file
		for svcName, svc := range cf.Services {
			containerName := containerNameForService(project, svcName, svc)
			state.Services[svcName] = ServiceState{
				ContainerID: containerName,
			}
		}
	}

	// Stop services in reverse dependency order
	order, _ := DependencyOrder(cf)
	// Reverse
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}

	for _, svcName := range order {
		svcState, ok := state.Services[svcName]
		if !ok {
			continue
		}
		fmt.Printf("  Stopping %s\n", svcName)
		if err := o.eng.ContainerStop(ctx, svcState.ContainerID); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: stopping %s: %v\n", svcName, err)
		}
		fmt.Printf("  Removing %s\n", svcName)
		if err := o.eng.ContainerRemove(ctx, svcState.ContainerID, true); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: removing %s: %v\n", svcName, err)
		}
	}

	// Remove networks
	for _, net := range state.Networks {
		fmt.Printf("  Removing network %s\n", net)
		if err := o.eng.NetworkRemove(ctx, net); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: removing network %s: %v\n", net, err)
		}
	}

	// Also try default network
	defaultNet := project + "_default"
	_ = o.eng.NetworkRemove(ctx, defaultNet)

	// Remove project networks from compose file
	for netName := range cf.Networks {
		fullName := project + "_" + netName
		_ = o.eng.NetworkRemove(ctx, fullName)
	}

	// Remove volumes if requested
	if opts.Volumes {
		for _, vol := range state.Volumes {
			fmt.Printf("  Removing volume %s\n", vol)
			if err := o.eng.VolumeRemove(ctx, vol); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: removing volume %s: %v\n", vol, err)
			}
		}
	}

	// Clean up state
	_ = DeleteProject(project)
	fmt.Printf("\nProject %q is down\n", project)
	return nil
}

// PsOptions configures the ps command.
type PsOptions struct {
	File    string
	Project string
}

// Ps returns the status of all services in the project.
func (o *Orchestrator) Ps(ctx context.Context, opts PsOptions) ([]ServiceStatus, error) {
	_, absFile, err := Load(opts.File)
	if err != nil {
		return nil, err
	}

	project := opts.Project
	if project == "" {
		project = ProjectName(absFile)
	}

	state, err := LoadProject(project)
	if err != nil {
		return nil, fmt.Errorf("project %q not found (has it been started?)", project)
	}

	var result []ServiceStatus
	for svcName, svcState := range state.Services {
		status := o.getContainerStatus(ctx, svcState.ContainerID)
		if status == "" {
			status = "not found"
		}
		result = append(result, ServiceStatus{
			Service:   svcName,
			Container: svcState.ContainerID,
			Image:     svcState.Image,
			Status:    status,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Service < result[j].Service
	})

	return result, nil
}

// ServiceStatus is used for ps output.
type ServiceStatus struct {
	Service   string `json:"service"`
	Container string `json:"container"`
	Image     string `json:"image"`
	Status    string `json:"status"`
}

// LogsOptions configures the logs command.
type LogsOptions struct {
	File    string
	Project string
	Service string // empty = all services
	Follow  bool
}

// Logs shows logs for one or all services.
func (o *Orchestrator) Logs(ctx context.Context, opts LogsOptions) error {
	_, absFile, err := Load(opts.File)
	if err != nil {
		return err
	}

	project := opts.Project
	if project == "" {
		project = ProjectName(absFile)
	}

	state, err := LoadProject(project)
	if err != nil {
		return fmt.Errorf("project %q not found", project)
	}

	if opts.Service != "" {
		svcState, ok := state.Services[opts.Service]
		if !ok {
			return fmt.Errorf("service %q not found in project", opts.Service)
		}
		return o.eng.ContainerLogs(ctx, svcState.ContainerID, opts.Follow)
	}

	// Show logs for all services (non-follow only for multi-service)
	for svcName, svcState := range state.Services {
		fmt.Printf("=== %s ===\n", svcName)
		if err := o.eng.ContainerLogs(ctx, svcState.ContainerID, false); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: logs for %s: %v\n", svcName, err)
		}
		fmt.Println()
	}
	return nil
}

// RestartOptions configures the restart command.
type RestartOptions struct {
	File    string
	Project string
	Service string
}

// Restart stops and starts a service (or all services).
func (o *Orchestrator) Restart(ctx context.Context, opts RestartOptions) error {
	_, absFile, err := Load(opts.File)
	if err != nil {
		return err
	}

	project := opts.Project
	if project == "" {
		project = ProjectName(absFile)
	}

	state, err := LoadProject(project)
	if err != nil {
		return fmt.Errorf("project %q not found", project)
	}

	services := make(map[string]ServiceState)
	if opts.Service != "" {
		svcState, ok := state.Services[opts.Service]
		if !ok {
			return fmt.Errorf("service %q not found in project", opts.Service)
		}
		services[opts.Service] = svcState
	} else {
		services = state.Services
	}

	for svcName, svcState := range services {
		fmt.Printf("  Restarting %s\n", svcName)
		if err := o.eng.ContainerStop(ctx, svcState.ContainerID); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: stopping %s: %v\n", svcName, err)
		}
		if err := o.eng.ContainerStart(ctx, svcState.ContainerID); err != nil {
			return fmt.Errorf("starting %s: %w", svcName, err)
		}
		svcState.Status = "running"
		state.Services[svcName] = svcState
	}

	return SaveProject(state)
}

// BuildOptions configures the build command.
type BuildOptions struct {
	File    string
	Project string
}

// Build builds images for services that have a build directive.
func (o *Orchestrator) Build(ctx context.Context, opts BuildOptions) error {
	cf, absFile, err := Load(opts.File)
	if err != nil {
		return err
	}

	project := opts.Project
	if project == "" {
		project = ProjectName(absFile)
	}

	for svcName, svc := range cf.Services {
		if !svc.Build.IsSet() {
			continue
		}

		tag := svc.Image
		if tag == "" {
			tag = project + "-" + svcName
		}

		var args []string
		args = append(args, "-t", tag)
		if svc.Build.Dockerfile != "" {
			args = append(args, "-f", svc.Build.Dockerfile)
		}
		args = append(args, svc.Build.Context)

		fmt.Printf("Building %s (%s)...\n", svcName, tag)
		if err := o.eng.ImageBuild(ctx, args); err != nil {
			return fmt.Errorf("building %s: %w", svcName, err)
		}
	}
	return nil
}

func (o *Orchestrator) getContainerStatus(ctx context.Context, nameOrID string) string {
	data, err := o.eng.ContainerInspect(ctx, nameOrID)
	if err != nil {
		return ""
	}
	// Try to extract status from inspect JSON
	s := string(data)
	// Simple approach: look for status field
	for _, candidate := range []string{`"status":"`, `"Status":"`} {
		if idx := strings.Index(s, candidate); idx != -1 {
			start := idx + len(candidate)
			end := strings.Index(s[start:], `"`)
			if end != -1 {
				return s[start : start+end]
			}
		}
	}
	return "unknown"
}

// volumeDataDirEnv returns env vars that redirect data directories to a
// subdirectory of the volume mount, avoiding the ext4 lost+found problem.
// Only applies to known images that require empty data directories.
var volumeDataDirOverrides = map[string]struct {
	envVar   string
	mountDir string
}{
	"postgres": {envVar: "PGDATA", mountDir: "/var/lib/postgresql/data"},
	"mysql":    {envVar: "MYSQL_DATADIR", mountDir: "/var/lib/mysql"},
	"mariadb":  {envVar: "MYSQL_DATADIR", mountDir: "/var/lib/mysql"},
}

func volumeDataDirEnv(svc Service) map[string]string {
	result := make(map[string]string)
	for _, v := range svc.Volumes {
		if !isNamedVolume(v) {
			continue
		}
		target := namedVolumeMountTarget(v)
		for imagePrefix, override := range volumeDataDirOverrides {
			if strings.Contains(svc.Image, imagePrefix) && target == override.mountDir {
				result[override.envVar] = override.mountDir + "/data"
			}
		}
	}
	return result
}

func containerNameForService(project, svcName string, svc Service) string {
	if svc.ContainerName != "" {
		return svc.ContainerName
	}
	return project + "-" + svcName + "-1"
}

// resolveVolume maps named volumes to project-scoped container volumes.
// e.g., "dbdata:/var/lib/data" → "myproject_dbdata:/var/lib/data"
// Host paths (starting with . or /) are left unchanged.
func resolveVolume(project, spec string, cf *ComposeFile) string {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) < 2 {
		return spec
	}
	source := parts[0]
	// If it's a path (absolute or relative), don't prefix
	if strings.HasPrefix(source, "/") || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~") {
		return spec
	}
	// Named volume — use project-scoped container volume
	fullName := project + "_" + source
	return fullName + ":" + parts[1]
}

// isNamedVolume checks whether a volume spec refers to a named volume (not a bind mount).
func isNamedVolume(spec string) bool {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) < 2 {
		return false
	}
	source := parts[0]
	return !strings.HasPrefix(source, "/") && !strings.HasPrefix(source, ".") && !strings.HasPrefix(source, "~")
}

// namedVolumeMountTarget returns the container-side mount target for a named volume spec.
func namedVolumeMountTarget(spec string) string {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}
