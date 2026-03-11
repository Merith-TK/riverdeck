package main

// This file re-exports the shared prober types and delegates ProbeDevice.
// All logic lives in pkg/prober.

import (
	"time"

	"github.com/merith-tk/riverdeck/pkg/prober"
	"github.com/sstallion/go-hid"
)

// Type aliases so the rest of this package can use the short names.
type ProbeResult = prober.ProbeResult
type FeatureReportResult = prober.FeatureReportResult
type DeviceCapabilities = prober.DeviceCapabilities
type KeyPacketInfo = prober.KeyPacketInfo
type CapturedKeyEvent = prober.CapturedKeyEvent

// ProbeDevice delegates to the shared prober package.
func ProbeDevice(raw hid.DeviceInfo, listenDur time.Duration, allReports bool) ProbeResult {
	return prober.ProbeDevice(raw, listenDur, allReports)
}
