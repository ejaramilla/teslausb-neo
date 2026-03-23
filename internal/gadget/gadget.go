package gadget

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	configFSRoot    = "/sys/kernel/config/usb_gadget"
	defaultIDVendor  = "0x1d6b" // Linux Foundation
	defaultIDProduct = "0x0104" // Multifunction Composite Gadget
	defaultBcdUSB    = "0x0200"
	defaultBcdDevice = "0x0100"
	manufacturer     = "Tesla"
	product          = "USB Drive"
	serialNumber     = "000000000001"
	maxPower         = "250"
	configuration    = "TeslaUSB"
)

// LUN represents a logical unit within a USB mass storage gadget.
type LUN struct {
	Device       string
	InquiryString string
	Nofua        bool
}

// Gadget manages a USB gadget via configfs.
type Gadget struct {
	root   string
	name   string
	LUNs   []LUN
	active bool
}

// New creates a new Gadget manager. The name parameter is the gadget directory
// name under configfs (e.g. "teslausb").
func New(name string) *Gadget {
	return &Gadget{
		root: configFSRoot,
		name: name,
	}
}

// gadgetPath returns the full configfs path for this gadget.
func (g *Gadget) gadgetPath() string {
	return filepath.Join(g.root, g.name)
}

// Configure creates the configfs directory structure for the USB gadget
// including device descriptors, strings, and mass storage function.
func (g *Gadget) Configure() error {
	gp := g.gadgetPath()

	if err := os.MkdirAll(gp, 0755); err != nil {
		return fmt.Errorf("create gadget dir: %w", err)
	}

	descriptors := map[string]string{
		"idVendor":  defaultIDVendor,
		"idProduct": defaultIDProduct,
		"bcdUSB":    defaultBcdUSB,
		"bcdDevice": defaultBcdDevice,
	}
	for name, value := range descriptors {
		if err := writeFile(filepath.Join(gp, name), value); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}

	// English strings (0x409)
	stringsDir := filepath.Join(gp, "strings", "0x409")
	if err := os.MkdirAll(stringsDir, 0755); err != nil {
		return fmt.Errorf("create strings dir: %w", err)
	}
	stringsMap := map[string]string{
		"manufacturer": manufacturer,
		"product":      product,
		"serialnumber": serialNumber,
	}
	for name, value := range stringsMap {
		if err := writeFile(filepath.Join(stringsDir, name), value); err != nil {
			return fmt.Errorf("write string %s: %w", name, err)
		}
	}

	// Configuration
	configDir := filepath.Join(gp, "configs", "c.1")
	if err := os.MkdirAll(filepath.Join(configDir, "strings", "0x409"), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := writeFile(filepath.Join(configDir, "MaxPower"), maxPower); err != nil {
		return fmt.Errorf("write MaxPower: %w", err)
	}
	if err := writeFile(filepath.Join(configDir, "strings", "0x409", "configuration"), configuration); err != nil {
		return fmt.Errorf("write configuration string: %w", err)
	}

	// Mass storage function
	funcDir := filepath.Join(gp, "functions", "mass_storage.usb0")
	if err := os.MkdirAll(funcDir, 0755); err != nil {
		return fmt.Errorf("create function dir: %w", err)
	}
	if err := writeFile(filepath.Join(funcDir, "stall"), "1"); err != nil {
		return fmt.Errorf("write stall: %w", err)
	}

	return nil
}

// AddLUN adds a logical unit backed by the given device path. The inquiry
// string is the name reported to the USB host.
func (g *Gadget) AddLUN(device, inquiryString string) error {
	lunIndex := len(g.LUNs)
	lunDir := filepath.Join(g.gadgetPath(), "functions", "mass_storage.usb0", fmt.Sprintf("lun.%d", lunIndex))

	if err := os.MkdirAll(lunDir, 0755); err != nil {
		return fmt.Errorf("create lun.%d dir: %w", lunIndex, err)
	}

	if err := writeFile(filepath.Join(lunDir, "file"), device); err != nil {
		return fmt.Errorf("write lun file: %w", err)
	}
	if err := writeFile(filepath.Join(lunDir, "inquiry_string"), inquiryString); err != nil {
		return fmt.Errorf("write inquiry_string: %w", err)
	}
	if err := writeFile(filepath.Join(lunDir, "removable"), "1"); err != nil {
		return fmt.Errorf("write removable: %w", err)
	}
	if err := writeFile(filepath.Join(lunDir, "ro"), "0"); err != nil {
		return fmt.Errorf("write ro: %w", err)
	}
	if err := writeFile(filepath.Join(lunDir, "nofua"), "0"); err != nil {
		return fmt.Errorf("write nofua: %w", err)
	}

	g.LUNs = append(g.LUNs, LUN{
		Device:       device,
		InquiryString: inquiryString,
		Nofua:        false,
	})

	return nil
}

// SetNofua enables or disables Force Unit Access for the specified LUN.
func (g *Gadget) SetNofua(lun int, enabled bool) error {
	if lun < 0 || lun >= len(g.LUNs) {
		return fmt.Errorf("lun index %d out of range (have %d LUNs)", lun, len(g.LUNs))
	}

	val := "0"
	if enabled {
		val = "1"
	}

	lunDir := filepath.Join(g.gadgetPath(), "functions", "mass_storage.usb0", fmt.Sprintf("lun.%d", lun))
	if err := writeFile(filepath.Join(lunDir, "nofua"), val); err != nil {
		return fmt.Errorf("set nofua on lun.%d: %w", lun, err)
	}

	g.LUNs[lun].Nofua = enabled
	return nil
}

// Activate binds the gadget to the USB Device Controller, making it visible
// to the USB host.
func (g *Gadget) Activate() error {
	gp := g.gadgetPath()

	// Symlink function into configuration
	funcPath := filepath.Join(gp, "functions", "mass_storage.usb0")
	linkPath := filepath.Join(gp, "configs", "c.1", "mass_storage.usb0")

	// Create symlink if it doesn't already exist
	if _, err := os.Lstat(linkPath); os.IsNotExist(err) {
		if err := os.Symlink(funcPath, linkPath); err != nil {
			return fmt.Errorf("symlink function to config: %w", err)
		}
	}

	udc, err := findUDC()
	if err != nil {
		return fmt.Errorf("find UDC: %w", err)
	}

	if err := writeFile(filepath.Join(gp, "UDC"), udc); err != nil {
		return fmt.Errorf("write UDC: %w", err)
	}

	g.active = true
	return nil
}

// Deactivate unbinds the gadget from the UDC.
func (g *Gadget) Deactivate() error {
	if err := writeFile(filepath.Join(g.gadgetPath(), "UDC"), ""); err != nil {
		return fmt.Errorf("clear UDC: %w", err)
	}
	g.active = false
	return nil
}

// Destroy removes the gadget's configfs directory structure. The gadget must
// be deactivated first.
func (g *Gadget) Destroy() error {
	if g.active {
		if err := g.Deactivate(); err != nil {
			return fmt.Errorf("deactivate before destroy: %w", err)
		}
	}

	gp := g.gadgetPath()

	// Remove function symlink from config
	linkPath := filepath.Join(gp, "configs", "c.1", "mass_storage.usb0")
	_ = os.Remove(linkPath)

	// Remove LUN dirs
	for i := range g.LUNs {
		lunDir := filepath.Join(gp, "functions", "mass_storage.usb0", fmt.Sprintf("lun.%d", i))
		_ = os.RemoveAll(lunDir)
	}

	// Remove directories in reverse order of creation
	dirsToRemove := []string{
		filepath.Join(gp, "functions", "mass_storage.usb0"),
		filepath.Join(gp, "configs", "c.1", "strings", "0x409"),
		filepath.Join(gp, "configs", "c.1"),
		filepath.Join(gp, "strings", "0x409"),
		gp,
	}
	for _, d := range dirsToRemove {
		_ = os.Remove(d)
	}

	g.LUNs = nil
	return nil
}

// writeFile writes a string value to a file, creating it if necessary.
func writeFile(path, value string) error {
	return os.WriteFile(path, []byte(value), 0644)
}

// readFile reads the contents of a file and returns it trimmed.
func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// findUDC returns the name of the first available USB Device Controller.
func findUDC() (string, error) {
	entries, err := os.ReadDir("/sys/class/udc")
	if err != nil {
		return "", fmt.Errorf("read /sys/class/udc: %w", err)
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("no UDC found in /sys/class/udc")
	}
	return entries[0].Name(), nil
}
