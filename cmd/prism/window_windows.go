package main

import "github.com/wailsapp/wails/v3/pkg/application"

// makeTransparent is a no-op on Windows: Wails sets up WebView2 transparency.
func makeTransparent(*application.WebviewWindow) {}
