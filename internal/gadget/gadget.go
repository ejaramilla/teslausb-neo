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
//
// It first tears down any gadget left behind by a previous (possibly crashed)
// run. configfs state survives a process crash, and function attributes such
// as "stall" cannot be rewritten while the gadget is bound to a UDC (the
// kernel returns EBUSY), so a clean teardown is required for the daemon to be
// restart-safe.
func (g *Gadget) Configure() error {
	gp := g.gadgetPath()

	g.teardownExisting()

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

	// If a UDC is already bound (e.g. we are re-activating, or a previous run
	// left it bound), writing a new value returns EBUSY. Treat an already-bound
	// matching UDC as success; otherwise clear it first.
	if cur, _ := readFile(filepath.Join(gp, "UDC")); cur != "" {
		if cur == udc {
			g.active = true
			return nil
		}
		_ = clearUDC(gp)
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

	g.removeConfigfsTree(g.gadgetPath())
	g.LUNs = nil
	return nil
}

// teardownExisting best-effort removes any pre-existing gadget at this path
// left over from a previous run. Safe to call when no gadget exists.
func (g *Gadget) teardownExisting() {
	gp := g.gadgetPath()
	if _, err := os.Stat(gp); err != nil {
		return // nothing there
	}
	_ = clearUDC(gp)
	g.removeConfigfsTree(gp)
}

// removeConfigfsTree unbinds and removes the full configfs structure for the
// gadget at gp. lun.* directories are discovered by globbing so that LUNs
// created by a previous process (not tracked in g.LUNs) are also removed.
// configfs requires children be removed before parents, hence the ordering.
func (g *Gadget) removeConfigfsTree(gp string) {
	// Unlink functions from the config first (a bound/linked function cannot
	// be removed).
	if links, err := filepath.Glob(filepath.Join(gp, "configs", "c.1", "*.usb0")); err == nil {
		for _, l := range links {
			_ = os.Remove(l)
		}
	}
	// Remove all LUN directories (globbed, not just g.LUNs).
	if luns, err := filepath.Glob(filepath.Join(gp, "functions", "mass_storage.usb0", "lun.*")); err == nil {
		for _, l := range luns {
			_ = os.Remove(l)
		}
	}
	// Deepest-first, parents after their children. On configfs the default
	// group dirs (functions, configs, strings) are removed implicitly with the
	// gadget and these explicit rmdirs simply no-op; on a normal filesystem
	// they collapse the whole tree.
	dirsToRemove := []string{
		filepath.Join(gp, "functions", "mass_storage.usb0"),
		filepath.Join(gp, "functions"),
		filepath.Join(gp, "configs", "c.1", "strings", "0x409"),
		filepath.Join(gp, "configs", "c.1", "strings"),
		filepath.Join(gp, "configs", "c.1"),
		filepath.Join(gp, "configs"),
		filepath.Join(gp, "strings", "0x409"),
		filepath.Join(gp, "strings"),
		gp,
	}
	for _, d := range dirsToRemove {
		_ = os.Remove(d)
	}
}

// clearUDC unbinds the gadget from its UDC if one is bound. Writing an empty
// string to the UDC attribute is idempotent (no-op when already unbound).
func clearUDC(gp string) error {
	udcPath := filepath.Join(gp, "UDC")
	if cur, err := readFile(udcPath); err != nil || cur == "" {
		return nil
	}
	return writeFile(udcPath, "")
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
	// Prefer a real hardware controller over a dummy_hcd test controller if
	// both are present.
	for _, e := range entries {
		if !strings.Contains(e.Name(), "dummy") {
			return e.Name(), nil
		}
	}
	return entries[0].Name(), nil
}
