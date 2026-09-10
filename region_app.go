package main

import "colorninja/internal/studio"

func (a *App) RegionEditor(id, revision uint64) (*studio.RegionState, error) {
	return a.studio.RegionEditor(id, revision)
}
func (a *App) EditRegions(command studio.RegionCommand) (*studio.RegionState, error) {
	return a.studio.EditRegions(command)
}
