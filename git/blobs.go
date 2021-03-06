package git

import (
	"github.com/gitql/gitql/sql"

	"gopkg.in/src-d/go-git.v4"
)

type blobsTable struct {
	r *git.Repository
}

func newBlobsTable(r *git.Repository) sql.Table {
	return &blobsTable{r: r}
}

func (blobsTable) Resolved() bool {
	return true
}

func (blobsTable) Name() string {
	return blobsTableName
}

func (blobsTable) Schema() sql.Schema {
	return sql.Schema{
		sql.Field{"hash", sql.String},
		sql.Field{"size", sql.BigInteger},
	}
}

func (r *blobsTable) TransformUp(f func(sql.Node) sql.Node) sql.Node {
	return f(r)
}

func (r *blobsTable) TransformExpressionsUp(f func(sql.Expression) sql.Expression) sql.Node {
	return r
}

func (r blobsTable) RowIter() (sql.RowIter, error) {
	bIter, err := r.r.Blobs()
	if err != nil {
		return nil, err
	}
	iter := &blobIter{i: bIter}
	return iter, nil
}

func (blobsTable) Children() []sql.Node {
	return []sql.Node{}
}

type blobIter struct {
	i *git.BlobIter
}

func (i *blobIter) Next() (sql.Row, error) {
	blob, err := i.i.Next()
	if err != nil {
		return nil, err
	}

	return blobToRow(blob), nil
}

func blobToRow(c *git.Blob) sql.Row {
	return sql.NewMemoryRow(
		c.Hash.String(),
		c.Size,
	)
}
