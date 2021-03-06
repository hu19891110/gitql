package gitql

import (
	"github.com/gitql/gitql/sql"
	"github.com/gitql/gitql/sql/analyzer"
	"github.com/gitql/gitql/sql/parse"
)

type Engine struct {
	Catalog  *sql.Catalog
	Analyzer *analyzer.Analyzer
}

func New() *Engine {
	c := &sql.Catalog{}
	a := analyzer.New(c)
	return &Engine{c, a}
}

func (e *Engine) AddDatabase(db sql.Database) {
	e.Catalog.Databases = append(e.Catalog.Databases, db)
	e.Analyzer.CurrentDatabase = db.Name()
}

func (e *Engine) Query(query string) (sql.Schema, sql.RowIter, error) {
	parsed, err := parse.Parse(query)
	if err != nil {
		return nil, nil, err
	}

	analyzed, err := e.Analyzer.Analyze(parsed)
	if err != nil {
		return nil, nil, err
	}

	iter, err := analyzed.RowIter()
	if err != nil {
		return nil, nil, err
	}

	return analyzed.Schema(), iter, nil
}
