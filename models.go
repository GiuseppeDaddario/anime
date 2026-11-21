package main

type myListItem struct {
	title        string
	desc         string
	coverUrl     string
	searchResult *SearchResult
}

func (i myListItem) Title() string       { return i.title }
func (i myListItem) Description() string { return i.desc }
func (i myListItem) FilterValue() string { return i.title }
