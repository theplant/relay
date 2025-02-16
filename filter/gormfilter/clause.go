package gormfilter

import (
	"strings"

	"gorm.io/gorm/clause"
)

// ClauseNot builds a NOT clause
// This is a copy of gorm.io/gorm/clause.Not , but with clause.OrWithSpace
// https://github.com/go-gorm/gorm/pull/7371
func ClauseNot(exprs ...clause.Expression) clause.Expression {
	if len(exprs) == 0 {
		return nil
	}
	if len(exprs) == 1 {
		if andCondition, ok := exprs[0].(clause.AndConditions); ok {
			exprs = andCondition.Exprs
		}
	}
	return NotConditions{Exprs: exprs}
}

type NotConditions struct {
	Exprs []clause.Expression
}

func (not NotConditions) Build(builder clause.Builder) {
	anyNegationBuilder := false
	for _, c := range not.Exprs {
		if _, ok := c.(clause.NegationExpressionBuilder); ok {
			anyNegationBuilder = true
			break
		}
	}

	if anyNegationBuilder {
		if len(not.Exprs) > 1 {
			builder.WriteByte('(')
		}

		for idx, c := range not.Exprs {
			if idx > 0 {
				builder.WriteString(clause.OrWithSpace)
			}

			if negationBuilder, ok := c.(clause.NegationExpressionBuilder); ok {
				negationBuilder.NegationBuild(builder)
			} else {
				builder.WriteString("NOT ")
				e, wrapInParentheses := c.(clause.Expr)
				if wrapInParentheses {
					sql := strings.ToUpper(e.SQL)
					if wrapInParentheses = strings.Contains(sql, clause.AndWithSpace) || strings.Contains(sql, clause.OrWithSpace); wrapInParentheses {
						builder.WriteByte('(')
					}
				}

				c.Build(builder)

				if wrapInParentheses {
					builder.WriteByte(')')
				}
			}
		}

		if len(not.Exprs) > 1 {
			builder.WriteByte(')')
		}
	} else {
		builder.WriteString("NOT ")
		if len(not.Exprs) > 1 {
			builder.WriteByte('(')
		}

		for idx, c := range not.Exprs {
			if idx > 0 {
				switch c.(type) {
				case clause.OrConditions:
					builder.WriteString(clause.OrWithSpace)
				default:
					builder.WriteString(clause.AndWithSpace)
				}
			}

			e, wrapInParentheses := c.(clause.Expr)
			if wrapInParentheses {
				sql := strings.ToUpper(e.SQL)
				if wrapInParentheses = strings.Contains(sql, clause.AndWithSpace) || strings.Contains(sql, clause.OrWithSpace); wrapInParentheses {
					builder.WriteByte('(')
				}
			}

			c.Build(builder)

			if wrapInParentheses {
				builder.WriteByte(')')
			}
		}

		if len(not.Exprs) > 1 {
			builder.WriteByte(')')
		}
	}
}
