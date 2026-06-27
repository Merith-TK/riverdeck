package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewEmpty(t *testing.T) {
	l := NewEmpty()
	if len(l.Pages) != 1 {
		t.Fatalf("NewEmpty: got %d pages, want 1", len(l.Pages))
	}
	if l.Pages[0].Name != "Main" {
		t.Errorf("NewEmpty: page name = %q, want %q", l.Pages[0].Name, "Main")
	}
	if len(l.Pages[0].Buttons) != 0 {
		t.Errorf("NewEmpty: got %d buttons, want 0", len(l.Pages[0].Buttons))
	}
}

func TestLayoutPage_ButtonBySlot(t *testing.T) {
	page := LayoutPage{
		Buttons: []LayoutButton{
			{Slot: 0, Label: "Home", Action: "home"},
			{Slot: 5, Label: "Media", Action: "page", TargetPage: "Media"},
		},
	}

	btn := page.ButtonBySlot(0)
	if btn == nil {
		t.Fatal("ButtonBySlot(0) returned nil")
	}
	if btn.Label != "Home" {
		t.Errorf("ButtonBySlot(0).Label = %q, want %q", btn.Label, "Home")
	}

	btn = page.ButtonBySlot(1)
	if btn != nil {
		t.Errorf("ButtonBySlot(1) = %+v, want nil", btn)
	}

	btn = page.ButtonBySlot(5)
	if btn == nil {
		t.Fatal("ButtonBySlot(5) returned nil")
	}
	if btn.Action != "page" {
		t.Errorf("ButtonBySlot(5).Action = %q, want %q", btn.Action, "page")
	}
}

func TestLayout_PageByName(t *testing.T) {
	l := &Layout{
		Pages: []LayoutPage{
			{Name: "Main", Buttons: nil},
			{Name: "Media", Buttons: nil},
		},
	}

	p := l.PageByName("Main")
	if p == nil {
		t.Fatal("PageByName('Main') returned nil")
	}

	p = l.PageByName("Nonexistent")
	if p != nil {
		t.Errorf("PageByName('Nonexistent') = %+v, want nil", p)
	}

	p = l.PageByName("media")
	if p != nil {
		t.Errorf("PageByName('media') = %+v, want nil (case-sensitive)", p)
	}
}

func TestLayout_PageIndexByName(t *testing.T) {
	l := &Layout{
		Pages: []LayoutPage{
			{Name: "Main"},
			{Name: "Media"},
			{Name: "Settings"},
		},
	}

	if idx := l.PageIndexByName("Main"); idx != 0 {
		t.Errorf("PageIndexByName('Main') = %d, want 0", idx)
	}
	if idx := l.PageIndexByName("Media"); idx != 1 {
		t.Errorf("PageIndexByName('Media') = %d, want 1", idx)
	}
	if idx := l.PageIndexByName("Unknown"); idx != -1 {
		t.Errorf("PageIndexByName('Unknown') = %d, want -1", idx)
	}
}

func TestValidate_Valid(t *testing.T) {
	l := &Layout{
		Pages: []LayoutPage{
			{
				Name: "Main",
				Buttons: []LayoutButton{
					{Slot: 0, Label: "SET", Action: "home"},
					{Slot: 1, Label: "Test", Script: "test.lua"},
				},
			},
		},
	}
	errs := Validate(l)
	if len(errs) > 0 {
		t.Errorf("Validate returned errors: %v", errs)
	}
}

func TestValidate_NoPages(t *testing.T) {
	l := &Layout{Pages: nil}
	errs := Validate(l)
	if len(errs) == 0 {
		t.Fatal("Validate expected errors, got none")
	}
	if errs[0] != "layout has no pages" {
		t.Errorf("Validate error = %q, want %q", errs[0], "layout has no pages")
	}
}

func TestValidate_MissingHome(t *testing.T) {
	l := &Layout{
		Pages: []LayoutPage{
			{
				Name: "Main",
				Buttons: []LayoutButton{
					{Slot: 0, Label: "Back", Action: "back"},
				},
			},
		},
	}
	errs := Validate(l)
	found := false
	for _, e := range errs {
		if e == `page "Main" is missing a SET/HOME button (action: "home")` {
			found = true
		}
	}
	if !found {
		t.Errorf("Validate missing home: got %v, want error about missing home", errs)
	}
}

func TestValidate_MultipleHome(t *testing.T) {
	l := &Layout{
		Pages: []LayoutPage{
			{
				Name: "Main",
				Buttons: []LayoutButton{
					{Slot: 0, Label: "Home1", Action: "home"},
					{Slot: 7, Label: "Home2", Action: "home"},
				},
			},
		},
	}
	errs := Validate(l)
	found := false
	for _, e := range errs {
		if e == `page "Main" has 2 SET/HOME buttons (action: "home"); exactly one required` {
			found = true
		}
	}
	if !found {
		t.Errorf("Validate multiple home: got %v, want error about multiple home", errs)
	}
}

func TestValidate_UnnamedPage(t *testing.T) {
	l := &Layout{
		Pages: []LayoutPage{
			{
				Name: "",
				Buttons: []LayoutButton{
					{Slot: 0, Label: "Home", Action: "home"},
				},
			},
		},
	}
	errs := Validate(l)
	for _, e := range errs {
		t.Logf("error: %s", e)
	}
}

func TestLayoutPath(t *testing.T) {
	path := LayoutPath("/tmp/riverdeck")
	want := "/tmp/riverdeck/layout.json"
	if path != want {
		t.Errorf("LayoutPath = %q, want %q", path, want)
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Error("Exists on empty dir should be false")
	}
	if err := os.WriteFile(filepath.Join(dir, "layout.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir) {
		t.Error("Exists after creating layout.json should be true")
	}
}

func TestSaveAndLoadLayout(t *testing.T) {
	dir := t.TempDir()
	lay := &Layout{
		Pages: []LayoutPage{
			{
				Name: "Main",
				Buttons: []LayoutButton{
					{Slot: 0, Label: "SET", Action: "home"},
					{Slot: 2, Label: "Clock", Script: "clock.lua"},
				},
			},
		},
	}

	if err := SaveLayout(dir, "default", lay); err != nil {
		t.Fatalf("SaveLayout: %v", err)
	}

	loaded, err := LoadForDevice(dir, "")
	if err != nil {
		t.Fatalf("LoadForDevice: %v", err)
	}
	if len(loaded.Pages) != 1 {
		t.Fatalf("LoadForDevice pages = %d, want 1", len(loaded.Pages))
	}
	if loaded.Pages[0].Name != "Main" {
		t.Errorf("Loaded page name = %q, want %q", loaded.Pages[0].Name, "Main")
	}
	if len(loaded.Pages[0].Buttons) != 2 {
		t.Errorf("Loaded buttons = %d, want 2", len(loaded.Pages[0].Buttons))
	}
}

func TestSaveLayout_PreservesOtherLayouts(t *testing.T) {
	dir := t.TempDir()

	// Save gaming layout first
	gaming := &Layout{
		Pages: []LayoutPage{
			{Name: "Game", Buttons: []LayoutButton{
				{Slot: 0, Label: "SET", Action: "home"},
			}},
		},
	}
	if err := SaveLayout(dir, "gaming", gaming); err != nil {
		t.Fatalf("SaveLayout gaming: %v", err)
	}

	// Now save default layout — should preserve gaming
	defaultLay := &Layout{
		Pages: []LayoutPage{
			{Name: "Main", Buttons: []LayoutButton{
				{Slot: 0, Label: "SET", Action: "home"},
				{Slot: 1, Label: "Clock", Script: "clock.lua"},
			}},
		},
	}
	if err := SaveLayout(dir, "default", defaultLay); err != nil {
		t.Fatalf("SaveLayout default: %v", err)
	}

	// Load game layout - should still exist
	loaded, err := LoadForDevice(dir, "SERIAL-GAMING")
	if err != nil {
		t.Fatalf("LoadForDevice: %v", err)
	}
	// Default device with no mapping should get "default" layout
	if loaded.Pages[0].Name != "Main" {
		t.Errorf("Unmapped device got page %q, want %q", loaded.Pages[0].Name, "Main")
	}

	// Assign a device to gaming layout
	if err := AssignDeviceLayout(dir, "SERIAL-GAMING", "gaming"); err != nil {
		t.Fatalf("AssignDeviceLayout: %v", err)
	}

	loaded, err = LoadForDevice(dir, "SERIAL-GAMING")
	if err != nil {
		t.Fatalf("LoadForDevice: %v", err)
	}
	if loaded.Pages[0].Name != "Game" {
		t.Errorf("Assigned device got page %q, want %q", loaded.Pages[0].Name, "Game")
	}
}

func TestLoadForDevice_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	lay, err := LoadForDevice(dir, "")
	if err != nil {
		t.Fatalf("LoadForDevice on empty dir: %v", err)
	}
	if len(lay.Pages) != 1 || lay.Pages[0].Name != "Main" {
		t.Errorf("LoadForDevice empty = %+v, want empty Main page", lay)
	}
}

func TestLoadFile_OldFormatPromotion(t *testing.T) {
	dir := t.TempDir()
	oldData := `{"pages": [{"name": "Old", "buttons": [{"slot": 0, "label": "SET", "action": "home"}]}]}`
	if err := os.WriteFile(filepath.Join(dir, "layout.json"), []byte(oldData), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := LoadFile(dir)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if f == nil {
		t.Fatal("LoadFile returned nil")
	}
	if f.Layouts == nil {
		t.Fatal("Old format: Layouts is nil after promotion")
	}
	def, ok := f.Layouts["default"]
	if !ok {
		t.Fatal("Old format: no 'default' layout after promotion")
	}
	if len(def.Pages) != 1 || def.Pages[0].Name != "Old" {
		t.Errorf("Old format: promoted page = %+v", def.Pages[0])
	}
	if len(f.Pages) != 0 {
		t.Errorf("Old format: Pages should be nil after promotion, got %d pages", len(f.Pages))
	}
}

func TestSaveDeviceGeometry(t *testing.T) {
	dir := t.TempDir()
	g := &DeviceGeometry{
		ID:     "TEST001",
		Name:   "Test Device",
		Source: "hardware",
		Rows:   4,
		Cols:   8,
	}
	if err := SaveDeviceGeometry(dir, g); err != nil {
		t.Fatalf("SaveDeviceGeometry: %v", err)
	}

	geometries, err := LoadAllDeviceGeometries(dir)
	if err != nil {
		t.Fatalf("LoadAllDeviceGeometries: %v", err)
	}
	if len(geometries) != 1 {
		t.Fatalf("LoadAllDeviceGeometries returned %d geometries, want 1", len(geometries))
	}
	if geometries[0].ID != "TEST001" {
		t.Errorf("Geometry ID = %q, want %q", geometries[0].ID, "TEST001")
	}
}

func TestAssignDeviceLayout(t *testing.T) {
	dir := t.TempDir()

	// Save a layout first
	lay := &Layout{
		Pages: []LayoutPage{
			{Name: "Main", Buttons: []LayoutButton{
				{Slot: 0, Label: "SET", Action: "home"},
			}},
		},
	}
	if err := SaveLayout(dir, "default", lay); err != nil {
		t.Fatal(err)
	}

	// Assign a device
	if err := AssignDeviceLayout(dir, "SERIAL-XL", "default"); err != nil {
		t.Fatalf("AssignDeviceLayout: %v", err)
	}

	loaded, err := LoadForDevice(dir, "SERIAL-XL")
	if err != nil {
		t.Fatalf("LoadForDevice: %v", err)
	}
	if len(loaded.Pages) != 1 {
		t.Errorf("Assigned device: got %d pages, want 1", len(loaded.Pages))
	}
}
