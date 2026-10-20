// Package exercise holds the exercise catalog: categories, exercises, and
// the embedded YAML loader.
package exercise

// Category is a group of exercises loaded from one YAML file.
type Category struct {
	ID          string     `yaml:"id"`
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Exercises   []Exercise `yaml:"exercises"`
}

// Exercise is one question with its reference answer and comparison options.
type Exercise struct {
	ID         string   `yaml:"id"`
	Title      string   `yaml:"title"`
	Question   string   `yaml:"question"`
	Tables     []string `yaml:"tables"`
	Ordered    bool     `yaml:"ordered"`
	Mutates    bool     `yaml:"mutates"`
	Verify     string   `yaml:"verify"`
	Hints      []string `yaml:"hints"`
	Answer     string   `yaml:"answer"`
	Discussion string   `yaml:"discussion"`

	// Category is the owning category id, filled in by the loader.
	Category string `yaml:"-"`
}

// Key is the stable "<category>/<exercise>" identifier used for progress and drafts.
func (e Exercise) Key() string { return e.Category + "/" + e.ID }

// Catalog is every category in filename order.
type Catalog struct {
	Categories []Category
	// Warnings are non-fatal validation notes (e.g. ordered without "order by").
	Warnings []string
}

// Find looks up an exercise by "<category>/<exercise>" key.
func (c *Catalog) Find(key string) (Exercise, bool) {
	for _, cat := range c.Categories {
		for _, ex := range cat.Exercises {
			if ex.Key() == key {
				return ex, true
			}
		}
	}
	return Exercise{}, false
}
