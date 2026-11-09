package main

import "testing"

func TestFixtureCannotUseBundledPhysicalPhoneTools(t *testing.T) {
	for _, tools := range [][2]string{{"", ""}, {"mock-fastboot", ""}, {"", "mock-adb"}} {
		if fixtureIOAllowed("ci-test", false, tools[0], tools[1]) {
			t.Fatal("fake ROM permitted phone I/O without both explicit mock overrides")
		}
	}
	if !fixtureIOAllowed("ci-test", false, "mock-fastboot", "mock-adb") {
		t.Fatal("mock phone flow refused")
	}
	if !fixtureIOAllowed("ci-test", true, "", "") {
		t.Fatal("read-only fixture inspection refused")
	}
	if !fixtureIOAllowed("release", false, "", "") {
		t.Fatal("production flasher refused its verified bundled tools")
	}
}
