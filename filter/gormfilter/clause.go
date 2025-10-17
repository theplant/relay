package gormfilter

import (
	"strings"

	"gorm.io/gorm/clause"
)

// ClauseNot builds a NOT clause that correctly handles De Morgan's laws.
// This implementation applies negation to expressions with proper OR/AND conversion.
//
// This is based on gorm.io/gorm/clause.Not but uses clause.OrWithSpace for proper negation.
// See: https://github.com/go-gorm/gorm/pull/7371
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

// NotConditions represents a NOT clause with support for proper negation of complex expressions.
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
			_ = builder.WriteByte('(')
		}

		for idx, c := range not.Exprs {
			if idx > 0 {
				_, _ = builder.WriteString(clause.OrWithSpace)
			}

			if negationBuilder, ok := c.(clause.NegationExpressionBuilder); ok {
				negationBuilder.NegationBuild(builder)
			} else {
				_, _ = builder.WriteString("NOT ")
				e, wrapInParentheses := c.(clause.Expr)
				if wrapInParentheses {
					sql := strings.ToUpper(e.SQL)
					if wrapInParentheses = strings.Contains(sql, clause.AndWithSpace) || strings.Contains(sql, clause.OrWithSpace); wrapInParentheses {
						_ = builder.WriteByte('(')
					}
				}

				c.Build(builder)

				if wrapInParentheses {
					_ = builder.WriteByte(')')
				}
			}
		}

		if len(not.Exprs) > 1 {
			_ = builder.WriteByte(')')
		}
	} else {
		_, _ = builder.WriteString("NOT ")
		if len(not.Exprs) > 1 {
			_ = builder.WriteByte('(')
		}

		for idx, c := range not.Exprs {
			if idx > 0 {
				switch c.(type) {
				case clause.OrConditions:
					_, _ = builder.WriteString(clause.OrWithSpace)
				default:
					_, _ = builder.WriteString(clause.AndWithSpace)
				}
			}

			e, wrapInParentheses := c.(clause.Expr)
			if wrapInParentheses {
				sql := strings.ToUpper(e.SQL)
				if wrapInParentheses = strings.Contains(sql, clause.AndWithSpace) || strings.Contains(sql, clause.OrWithSpace); wrapInParentheses {
					_ = builder.WriteByte('(')
				}
			}

			c.Build(builder)

			if wrapInParentheses {
				_ = builder.WriteByte(')')
			}
		}

		if len(not.Exprs) > 1 {
			_ = builder.WriteByte(')')
		}
	}
}
