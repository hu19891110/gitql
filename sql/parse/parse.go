package parse

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gitql/gitql/sql"
	"github.com/gitql/gitql/sql/expression"
	"github.com/gitql/gitql/sql/plan"

	"github.com/youtube/vitess/go/vt/sqlparser"
)

const (
	showTables = "SHOW TABLES"
)

func errUnsupported(n sqlparser.SQLNode) error {
	return fmt.Errorf("unsupported syntax: %#v", n)
}

func errUnsupportedFeature(feature string) error {
	return fmt.Errorf("unsupported feature: %s", feature)
}

func Parse(s string) (sql.Node, error) {
	if strings.HasSuffix(s, ";") {
		s = s[:len(s)-1]
	}

	// TODO implement it into the parser
	if strings.ToUpper(s) == showTables {
		return plan.NewShowTables(&sql.UnresolvedDatabase{}), nil
	}

	t := regexp.MustCompile(`^describe\s+table\s+(.*)`).FindStringSubmatch(strings.ToLower(s))
	if len(t) == 2 && t[1] != "" {
		return plan.NewDescribe(plan.NewUnresolvedTable(t[1])), nil
	}

	stmt, err := sqlparser.Parse(s)
	if err != nil {
		return nil, err
	}

	return convert(stmt)
}

func convert(stmt sqlparser.Statement) (sql.Node, error) {
	switch n := stmt.(type) {
	default:
		return nil, errUnsupported(n)
	case *sqlparser.Select:
		return convertSelect(n)
	}
}

func convertSelect(s *sqlparser.Select) (sql.Node, error) {
	var node sql.Node

	node, err := tableExprsToTable(s.From)
	if err != nil {
		return nil, err
	}

	if s.Distinct != "" {
		return nil, errUnsupportedFeature("DISTINCT")
	}

	if len(s.GroupBy) != 0 {
		return nil, errUnsupportedFeature("GROUP BY")
	}

	if s.Having != nil {
		return nil, errUnsupportedFeature("HAVING")
	}

	if s.Where != nil {
		node, err = whereToFilter(s.Where, node)
		if err != nil {
			return nil, err
		}
	}

	if len(s.OrderBy) != 0 {
		node, err = orderByToSort(s.OrderBy, node)
		if err != nil {
			return nil, err
		}
	}

	if s.Limit != nil {
		//TODO: Add support for offset
		node, err = limitToLimit(s.Limit.Rowcount, node)
		if err != nil {
			return nil, err
		}
	}

	node, err = selectToProject(s.SelectExprs, node)
	if err != nil {
		return nil, err
	}

	return node, nil
}

func tableExprsToTable(te sqlparser.TableExprs) (sql.Node, error) {
	if len(te) == 0 {
		return nil, errUnsupportedFeature("zero tables in FROM")
	}

	var nodes []sql.Node
	for _, t := range te {
		n, err := tableExprToTable(t)
		if err != nil {
			return nil, err
		}

		nodes = append(nodes, n)
	}

	if len(nodes) == 1 {
		return nodes[0], nil
	}

	if len(nodes) == 2 {
		return plan.NewCrossJoin(nodes[0], nodes[1]), nil
	}

	//TODO: Support N tables in JOIN.
	return nil, errUnsupportedFeature("more than 2 tables in JOIN")
}

func tableExprToTable(te sqlparser.TableExpr) (sql.Node, error) {
	switch t := (te).(type) {
	default:
		return nil, errUnsupported(te)
	case *sqlparser.AliasedTableExpr:
		//TODO: Add support for table alias.
		//TODO: Add support for qualifier.
		tn, ok := t.Expr.(*sqlparser.TableName)
		if !ok {
			return nil, errUnsupportedFeature("non simple tables")
		}

		return plan.NewUnresolvedTable(string(tn.Name)), nil
	}
}

func whereToFilter(w *sqlparser.Where, child sql.Node) (*plan.Filter, error) {
	c, err := boolExprToExpression(w.Expr)
	if err != nil {
		return nil, err
	}

	return plan.NewFilter(c, child), nil
}

func orderByToSort(ob sqlparser.OrderBy, child sql.Node) (*plan.Sort, error) {
	var sortFields []plan.SortField
	for _, o := range ob {
		e, err := valExprToExpression(o.Expr)
		if err != nil {
			return nil, err
		}

		var so plan.SortOrder
		switch o.Direction {
		default:
			panic(fmt.Errorf("invalid sort order: %s", o.Direction))
		case sqlparser.AscScr:
			so = plan.Ascending
		case sqlparser.DescScr:
			so = plan.Descending
		}

		sf := plan.SortField{Column: e, Order: so}
		sortFields = append(sortFields, sf)
	}

	return plan.NewSort(sortFields, child), nil
}

func limitToLimit(o sqlparser.ValExpr, child sql.Node) (*plan.Limit, error) {
	e, err := valExprToExpression(o)
	if err != nil {
		return nil, err
	}

	nl, ok := e.(*expression.Literal)
	if !ok || nl.Type() != sql.BigInteger {
		return nil, errUnsupportedFeature("LIMIT with non-integer literal")
	}

	n := (nl.Eval(nil)).(int64)
	return plan.NewLimit(n, child), nil
}

func selectToProject(se sqlparser.SelectExprs, child sql.Node) (*plan.Project, error) {
	var exprs []sql.Expression
	for _, e := range se {
		pe, err := selectExprToExpression(e)
		if err != nil {
			return nil, err
		}

		exprs = append(exprs, pe)
	}

	return plan.NewProject(exprs, child), nil
}

func exprToExpression(e sqlparser.Expr) (sql.Expression, error) {
	be, ok := e.(sqlparser.BoolExpr)
	if ok {
		return boolExprToExpression(be)
	}

	c, ok := e.(*sqlparser.ComparisonExpr)
	if ok {
		return comparisonExprToExpression(c)
	}

	v, ok := e.(sqlparser.ValExpr)
	if ok {
		return valExprToExpression(v)
	}

	return nil, errUnsupported(e)
}

func boolExprToExpression(be sqlparser.BoolExpr) (sql.Expression, error) {
	switch b := be.(type) {
	default:
		return nil, errUnsupported(b)
	case *sqlparser.ComparisonExpr:
		return comparisonExprToExpression(b)
	case *sqlparser.NotExpr:
		c, err := boolExprToExpression(b.Expr)
		if err != nil {
			return nil, err
		}

		return expression.NewNot(c), nil
	}
}

func comparisonExprToExpression(c *sqlparser.ComparisonExpr) (sql.Expression,
	error) {

	left, err := valExprToExpression(c.Left)
	if err != nil {
		return nil, err
	}

	right, err := valExprToExpression(c.Right)
	if err != nil {
		return nil, err
	}

	switch c.Operator {
	default:
		return nil, errUnsupportedFeature(c.Operator)
	case sqlparser.EqualStr:
		return expression.NewEquals(left, right), nil
	case sqlparser.LessThanStr:
		return expression.NewLessThan(left, right), nil
	case sqlparser.LessEqualStr:
		return expression.NewLessThanOrEqual(left, right), nil
	case sqlparser.GreaterThanStr:
		return expression.NewGreaterThan(left, right), nil
	case sqlparser.GreaterEqualStr:
		return expression.NewGreaterThanOrEqual(left, right), nil
	case sqlparser.NotEqualStr:
		return expression.NewNot(
			expression.NewEquals(left, right),
		), nil
	}
}

func valExprToExpression(ve sqlparser.ValExpr) (sql.Expression, error) {
	switch v := ve.(type) {
	default:
		return nil, errUnsupported(v)
	case sqlparser.StrVal:
		return expression.NewLiteral(string(v), sql.String), nil
	//TODO: case sqlparser.BoolVal:
	//	return expression.NewLiteral(bool(v), sql.Boolean), nil
	case sqlparser.NumVal:
		//TODO: Use smallest integer representation and widen later.
		n, _ := strconv.ParseInt(string(v), 10, 64)
		return expression.NewLiteral(n, sql.BigInteger), nil
	case sqlparser.HexVal:
		//TODO
		return nil, errUnsupported(v)
	case *sqlparser.NullVal:
		//TODO
		return expression.NewLiteral(nil, sql.Null), nil
	case *sqlparser.ColName:
		//TODO: add handling of case sensitiveness.
		return expression.NewUnresolvedColumn(v.Name.Lowered()), nil
	}
}

func selectExprToExpression(se sqlparser.SelectExpr) (sql.Expression, error) {
	switch e := se.(type) {
	default:
		return nil, errUnsupported(e)
	case *sqlparser.StarExpr:
		//TODO: Add support for qualified start.
		return expression.NewStar(), nil
	case *sqlparser.NonStarExpr:
		expr, err := exprToExpression(e.Expr)
		if err != nil {
			return nil, err
		}

		if e.As.String() == "" {
			return expr, nil
		}

		//TODO: Handle case-sensitiveness when needed.
		return expression.NewAlias(expr, e.As.Lowered()), nil
	}
}
