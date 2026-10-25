package autostart

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue = "PRism"
)

func enable(exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(runValue, `"`+exe+`" `+Arg); err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	return nil
}

func disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("autostart: %w", err)
	}
	defer k.Close()
	if err := k.DeleteValue(runValue); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("autostart: %w", err)
	}
	return nil
}

func enabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
