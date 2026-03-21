package compose

// ComposeFile represents a parsed docker-compose.yml.
type ComposeFile struct {
	Services map[string]Service `yaml:"services"`
	Networks map[string]Network `yaml:"networks,omitempty"`
	Volumes  map[string]Volume  `yaml:"volumes,omitempty"`
}

// Service represents a single service in a compose file.
type Service struct {
	Image         string            `yaml:"image"`
	ContainerName string            `yaml:"container_name,omitempty"`
	Command       CommandOrArgs     `yaml:"command,omitempty"`
	Entrypoint    CommandOrArgs     `yaml:"entrypoint,omitempty"`
	Ports         []string          `yaml:"ports,omitempty"`
	Volumes       []string          `yaml:"volumes,omitempty"`
	Environment   Environment       `yaml:"environment,omitempty"`
	EnvFile       StringOrSlice     `yaml:"env_file,omitempty"`
	DependsOn     DependsOn         `yaml:"depends_on,omitempty"`
	Networks      StringOrSlice     `yaml:"networks,omitempty"`
	Restart       string            `yaml:"restart,omitempty"`
	WorkingDir    string            `yaml:"working_dir,omitempty"`
	Hostname      string            `yaml:"hostname,omitempty"`
	Memory        string            `yaml:"mem_limit,omitempty"`
	CPUs          string            `yaml:"cpus,omitempty"`
	Labels        map[string]string `yaml:"labels,omitempty"`
}

// Network represents a top-level network definition.
type Network struct {
	Driver   string `yaml:"driver,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

// Volume represents a top-level volume definition.
type Volume struct {
	Driver   string `yaml:"driver,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

// ServiceState tracks a running service's container within a project.
type ServiceState struct {
	Service     string `json:"service"`
	ContainerID string `json:"container_id"`
	Image       string `json:"image"`
	Status      string `json:"status"` // running, stopped, created
}

// ProjectState is the persisted state of a compose project.
type ProjectState struct {
	Name     string                  `json:"name"`
	Dir      string                  `json:"dir"`
	File     string                  `json:"file"`
	Services map[string]ServiceState `json:"services"`
	Networks []string                `json:"networks,omitempty"`
	Volumes  []string                `json:"volumes,omitempty"`
}
