package workshop

type MountType int

const (
	HostWorkshop MountType = iota
	WorkshopWorkshop
	Volume
)

type Camera struct {
	Name string `json:"name"`
}

type Mount struct {
	Name  string    `json:"name"`
	What  string    `json:"what"`
	Where string    `json:"where"`
	Type  MountType `json:"type"`
}

type Proxy struct {
	Name    string
	Connect string
	Listen  string
	// A more flexible alternative could either be a func() []string to generate the environment,
	// or two embedded functions for the install and remove hooks. I don't think this will be difficult to change
	// later if required, so I haven't implemented anything complex (I don't like to overengineer prematurely)
	Env []string
}

type Gpu struct {
	Name string
}

type SdkProfile struct {
	Sdk string

	Camera  *Camera
	Mounts  map[string]Mount
	Proxies map[string]Proxy
	Gpu     *Gpu
}

func NewSdkProfile(sdkName string) SdkProfile {
	return SdkProfile{
		Sdk:     sdkName,
		Mounts:  make(map[string]Mount),
		Proxies: make(map[string]Proxy),
	}
}
