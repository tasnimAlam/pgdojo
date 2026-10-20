package exercise

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadEmbedded(t *testing.T) {
	cat, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Categories) != 7 {
		t.Fatalf("want 7 categories, got %d", len(cat.Categories))
	}
	total, mutating := 0, 0
	for _, c := range cat.Categories {
		if len(c.Exercises) < 3 {
			t.Errorf("category %s has %d exercises, want >= 3", c.ID, len(c.Exercises))
		}
		for _, ex := range c.Exercises {
			total++
			if ex.Mutates {
				mutating++
			}
			if ex.Category != c.ID {
				t.Errorf("%s: Category not set", ex.ID)
			}
		}
	}
	if total < 21 {
		t.Errorf("want >= 21 exercises, got %d", total)
	}
	if mutating == 0 {
		t.Error("want at least one mutating exercise")
	}
	if len(cat.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", cat.Warnings)
	}
	if _, ok := cat.Find("joins/member-booking-starttimes"); !ok {
		t.Error("Find did not locate joins/member-booking-starttimes")
	}
}

func TestLoadFSOrderAndWarnings(t *testing.T) {
	fsys := fstest.MapFS{
		"02-b.yaml": {Data: []byte(validCat("b", "ordered: true"))},
		"01-a.yaml": {Data: []byte(validCat("a", ""))},
	}
	cat, err := LoadFS(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Categories[0].ID != "a" || cat.Categories[1].ID != "b" {
		t.Errorf("categories not sorted by filename: %s, %s", cat.Categories[0].ID, cat.Categories[1].ID)
	}
	if len(cat.Warnings) != 1 || !strings.Contains(cat.Warnings[0], "order by") {
		t.Errorf("want one order-by warning, got %v", cat.Warnings)
	}
}

func TestLoadFSReportsEveryProblem(t *testing.T) {
	bad := `id: x
title: X
exercises:
  - id: dup
    title: One
    question: ""
    hints: []
    answer: ""
  - id: dup
    title: Two
    question: q
    hints: [h]
    answer: select 1
    mutates: true
  - id: Not_Kebab
    title: Three
    question: q
    hints: [h]
    answer: select 1
`
	fsys := fstest.MapFS{
		"01-x.yaml": {Data: []byte(bad)},
		"02-x.yaml": {Data: []byte(validCat("x", ""))},
		"03-y.yaml": {Data: []byte("id: y\nbogus: 1\n")},
	}
	_, err := LoadFS(fsys)
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{
		"question is empty",
		"answer is empty",
		"needs at least one hint",
		"duplicate id",
		"requires a verify query",
		"kebab-case",
		`category id "x" already used`,
		"bogus",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q:\n%v", want, err)
		}
	}
}

func validCat(id, extra string) string {
	return "id: " + id + `
title: T
exercises:
  - id: e
    title: E
    question: q
    hints: [h]
    answer: select 1
    ` + extra + "\n"
}
