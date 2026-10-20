package exercise

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed data/*.yaml
var dataFS embed.FS

var (
	kebab   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	orderBy = regexp.MustCompile(`(?i)\border\s+by\b`)
)

// Load parses and validates the embedded exercise data.
func Load() (*Catalog, error) {
	sub, err := fs.Sub(dataFS, "data")
	if err != nil {
		return nil, err
	}
	return LoadFS(sub)
}

// LoadFS parses every *.yaml file at the root of fsys, sorted by filename,
// and returns an error listing every validation problem found.
func LoadFS(fsys fs.FS) (*Catalog, error) {
	names, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	cat := &Catalog{}
	var problems []string
	seenCat := map[string]string{}

	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		var c Category
		dec := yaml.NewDecoder(strings.NewReader(string(raw)))
		dec.KnownFields(true)
		if err := dec.Decode(&c); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		p, w := validate(name, &c)
		problems = append(problems, p...)
		cat.Warnings = append(cat.Warnings, w...)

		if c.ID != "" {
			if prev, ok := seenCat[c.ID]; ok {
				problems = append(problems, fmt.Sprintf("%s: category id %q already used in %s", name, c.ID, prev))
			}
			seenCat[c.ID] = name
		}
		for i := range c.Exercises {
			c.Exercises[i].Category = c.ID
		}
		cat.Categories = append(cat.Categories, c)
	}

	if len(problems) > 0 {
		return nil, errors.New("invalid exercise data:\n  " + strings.Join(problems, "\n  "))
	}
	return cat, nil
}

func validate(file string, c *Category) (problems, warnings []string) {
	add := func(format string, args ...any) {
		problems = append(problems, file+": "+fmt.Sprintf(format, args...))
	}
	if !kebab.MatchString(c.ID) {
		add("category id %q must be non-empty kebab-case", c.ID)
	}
	if strings.TrimSpace(c.Title) == "" {
		add("category %q has no title", c.ID)
	}
	if len(c.Exercises) == 0 {
		add("category %q has no exercises", c.ID)
	}

	seen := map[string]bool{}
	for i, ex := range c.Exercises {
		where := fmt.Sprintf("exercise #%d (%s)", i+1, ex.ID)
		if !kebab.MatchString(ex.ID) {
			add("%s: id must be non-empty kebab-case", where)
		} else if seen[ex.ID] {
			add("%s: duplicate id", where)
		}
		seen[ex.ID] = true
		if strings.TrimSpace(ex.Title) == "" {
			add("%s: title is empty", where)
		}
		if strings.TrimSpace(ex.Question) == "" {
			add("%s: question is empty", where)
		}
		if strings.TrimSpace(ex.Answer) == "" {
			add("%s: answer is empty", where)
		}
		if len(ex.Hints) == 0 {
			add("%s: needs at least one hint", where)
		}
		if ex.Mutates && strings.TrimSpace(ex.Verify) == "" {
			add("%s: mutates: true requires a verify query", where)
		}
		if ex.Ordered && !orderBy.MatchString(ex.Answer) {
			warnings = append(warnings, fmt.Sprintf("%s: %s: ordered: true but answer has no order by", file, where))
		}
	}
	return problems, warnings
}
