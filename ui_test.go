package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
)

func TestCatalogueLoadsInsideTheTUI(t *testing.T) {
	m := initModel(nil)
	m.state = stateLoading
	if !strings.Contains(m.View(), "Caricamento catalogo") || m.Init() == nil {
		t.Fatal("The loading screen must fetch the catalogue")
	}
	updated, _ := m.Update(catalogMsg{items: []list.Item{myListItem{title: "Naruto"}}})
	if loaded := updated.(model); loaded.state != stateList || len(loaded.list.Items()) != 1 {
		t.Fatal("The catalogue must replace the loading screen")
	}
	updated, cmd := m.Update(catalogMsg{err: fmt.Errorf("offline")})
	if updated.(model).err == nil || cmd == nil {
		t.Fatal("A load error must close the screen and be reported")
	}
}
