# System Status Icons

This directory contains system monitoring scripts for the Stream Deck interface.

## Available Icons

- **cpu.lua** - Shows CPU usage percentage (updates every 5 seconds)
  - Color-coded: Green (<60%), Orange (60-80%), Red (>80%)

- **memory.lua** - Shows memory usage percentage (updates every 5 seconds)
  - Color-coded: Green (<75%), Orange (75-90%), Red (>90%)

- **disk.lua** - Shows disk usage for root filesystem (updates every 30 seconds)
  - Color-coded: Green (<85%), Orange (85-95%), Red (>95%)

- **network.lua** - Shows network connectivity status (updates every 10 seconds)
  - Green: Online (can reach 8.8.8.8)
  - Red: Offline

- **uptime.lua** - Shows system uptime (updates every 60 seconds)
  - Shows days and hours if > 1 day
  - Shows hours and minutes if < 1 day

- **temperature.lua** - Shows CPU temperature (updates every 10 seconds)
  - Color-coded: Green (<55°C), Orange (55-70°C), Red (>70°C)
  - Supports Raspberry Pi and generic Linux thermal zones

- **shutdown.lua** - Safe system shutdown with confirmation

## Performance Optimizations

These scripts are optimized for low-power devices like the Raspberry Pi Zero 2W:

- **Reduced polling frequency**: System stats update every 5-60 seconds instead of continuously
- **Efficient commands**: Uses fast Linux system tools (`top`, `free`, `df`, `uptime`, etc.)
- **Passive updates**: Only run when buttons are visible (2fps update rate)
- **Caching**: Values are cached and only refreshed periodically

## Platform Support

- **Linux**: Full support with native commands
- **Raspberry Pi**: Special temperature monitoring support
- **Windows**: Legacy support (slower due to PowerShell overhead)