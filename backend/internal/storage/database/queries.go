package database

import (
	"fmt"
	"strings"
	"time"
)

type QueryBuilder struct {
	table      string
	columns    []string
	where      []WhereClause
	joins      []JoinClause
	orderBy    []OrderByClause
	groupBy    []string
	having     []WhereClause
	limit      int
	offset     int
	params     []interface{}
	paramCount int
}

type WhereClause struct {
	Column   string
	Operator string
	Value    interface{}
	Or       bool
}

type JoinClause struct {
	Type      string
	Table     string
	Condition string
}

type OrderByClause struct {
	Column string
	Desc   bool
}

type FilterOptions struct {
	Status    string
	Provider  string
	StartDate *time.Time
	EndDate   *time.Time
	Search    string
	SortBy    string
	SortOrder string
	Limit     int
	Offset    int
}

func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		columns:    []string{},
		where:      []WhereClause{},
		joins:      []JoinClause{},
		orderBy:    []OrderByClause{},
		groupBy:    []string{},
		having:     []WhereClause{},
		params:     []interface{}{},
		paramCount: 0,
	}
}

func (qb *QueryBuilder) Table(table string) *QueryBuilder {
	qb.table = table
	return qb
}

func (qb *QueryBuilder) Select(columns ...string) *QueryBuilder {
	qb.columns = columns
	return qb
}

func (qb *QueryBuilder) Where(column, operator string, value interface{}) *QueryBuilder {
	qb.where = append(qb.where, WhereClause{
		Column:   column,
		Operator: operator,
		Value:    value,
		Or:       false,
	})
	return qb
}

func (qb *QueryBuilder) OrWhere(column, operator string, value interface{}) *QueryBuilder {
	qb.where = append(qb.where, WhereClause{
		Column:   column,
		Operator: operator,
		Value:    value,
		Or:       true,
	})
	return qb
}

func (qb *QueryBuilder) WhereIn(column string, values []interface{}) *QueryBuilder {
	qb.where = append(qb.where, WhereClause{
		Column:   column,
		Operator: "IN",
		Value:    values,
		Or:       false,
	})
	return qb
}

func (qb *QueryBuilder) WhereNotIn(column string, values []interface{}) *QueryBuilder {
	qb.where = append(qb.where, WhereClause{
		Column:   column,
		Operator: "NOT IN",
		Value:    values,
		Or:       false,
	})
	return qb
}

func (qb *QueryBuilder) WhereBetween(column string, start, end interface{}) *QueryBuilder {
	qb.where = append(qb.where, WhereClause{
		Column:   column,
		Operator: "BETWEEN",
		Value:    []interface{}{start, end},
		Or:       false,
	})
	return qb
}

func (qb *QueryBuilder) WhereNull(column string) *QueryBuilder {
	qb.where = append(qb.where, WhereClause{
		Column:   column,
		Operator: "IS NULL",
		Value:    nil,
		Or:       false,
	})
	return qb
}

func (qb *QueryBuilder) WhereNotNull(column string) *QueryBuilder {
	qb.where = append(qb.where, WhereClause{
		Column:   column,
		Operator: "IS NOT NULL",
		Value:    nil,
		Or:       false,
	})
	return qb
}

func (qb *QueryBuilder) WhereLike(column string, pattern string) *QueryBuilder {
	qb.where = append(qb.where, WhereClause{
		Column:   column,
		Operator: "LIKE",
		Value:    pattern,
		Or:       false,
	})
	return qb
}

func (qb *QueryBuilder) Join(table, condition string) *QueryBuilder {
	qb.joins = append(qb.joins, JoinClause{
		Type:      "INNER JOIN",
		Table:     table,
		Condition: condition,
	})
	return qb
}

func (qb *QueryBuilder) LeftJoin(table, condition string) *QueryBuilder {
	qb.joins = append(qb.joins, JoinClause{
		Type:      "LEFT JOIN",
		Table:     table,
		Condition: condition,
	})
	return qb
}

func (qb *QueryBuilder) RightJoin(table, condition string) *QueryBuilder {
	qb.joins = append(qb.joins, JoinClause{
		Type:      "RIGHT JOIN",
		Table:     table,
		Condition: condition,
	})
	return qb
}

func (qb *QueryBuilder) OrderBy(column string, desc bool) *QueryBuilder {
	qb.orderBy = append(qb.orderBy, OrderByClause{
		Column: column,
		Desc:   desc,
	})
	return qb
}

func (qb *QueryBuilder) GroupBy(columns ...string) *QueryBuilder {
	qb.groupBy = append(qb.groupBy, columns...)
	return qb
}

func (qb *QueryBuilder) Having(column, operator string, value interface{}) *QueryBuilder {
	qb.having = append(qb.having, WhereClause{
		Column:   column,
		Operator: operator,
		Value:    value,
		Or:       false,
	})
	return qb
}

func (qb *QueryBuilder) Limit(limit int) *QueryBuilder {
	qb.limit = limit
	return qb
}

func (qb *QueryBuilder) Offset(offset int) *QueryBuilder {
	qb.offset = offset
	return qb
}

func (qb *QueryBuilder) BuildSelect() (string, []interface{}) {
	var query strings.Builder

	query.WriteString("SELECT ")

	if len(qb.columns) == 0 {
		query.WriteString("*")
	} else {
		query.WriteString(strings.Join(qb.columns, ", "))
	}

	query.WriteString(" FROM ")
	query.WriteString(qb.table)

	for _, join := range qb.joins {
		query.WriteString(" ")
		query.WriteString(join.Type)
		query.WriteString(" ")
		query.WriteString(join.Table)
		query.WriteString(" ON ")
		query.WriteString(join.Condition)
	}

	if len(qb.where) > 0 {
		query.WriteString(" WHERE ")
		qb.buildWhereClause(&query, qb.where)
	}

	if len(qb.groupBy) > 0 {
		query.WriteString(" GROUP BY ")
		query.WriteString(strings.Join(qb.groupBy, ", "))
	}

	if len(qb.having) > 0 {
		query.WriteString(" HAVING ")
		qb.buildWhereClause(&query, qb.having)
	}

	if len(qb.orderBy) > 0 {
		query.WriteString(" ORDER BY ")
		orderClauses := make([]string, len(qb.orderBy))
		for i, order := range qb.orderBy {
			if order.Desc {
				orderClauses[i] = order.Column + " DESC"
			} else {
				orderClauses[i] = order.Column + " ASC"
			}
		}
		query.WriteString(strings.Join(orderClauses, ", "))
	}

	if qb.limit > 0 {
		query.WriteString(fmt.Sprintf(" LIMIT %d", qb.limit))
	}

	if qb.offset > 0 {
		query.WriteString(fmt.Sprintf(" OFFSET %d", qb.offset))
	}

	return query.String(), qb.params
}

func (qb *QueryBuilder) BuildInsert(values map[string]interface{}) (string, []interface{}) {
	var query strings.Builder
	var columns []string
	var placeholders []string

	query.WriteString("INSERT INTO ")
	query.WriteString(qb.table)
	query.WriteString(" (")

	for column, value := range values {
		columns = append(columns, column)
		qb.paramCount++
		placeholders = append(placeholders, fmt.Sprintf("$%d", qb.paramCount))
		qb.params = append(qb.params, value)
	}

	query.WriteString(strings.Join(columns, ", "))
	query.WriteString(") VALUES (")
	query.WriteString(strings.Join(placeholders, ", "))
	query.WriteString(")")

	return query.String(), qb.params
}

func (qb *QueryBuilder) BuildBulkInsert(columns []string, rows [][]interface{}) (string, []interface{}) {
	var query strings.Builder

	query.WriteString("INSERT INTO ")
	query.WriteString(qb.table)
	query.WriteString(" (")
	query.WriteString(strings.Join(columns, ", "))
	query.WriteString(") VALUES ")

	valueClauses := make([]string, len(rows))
	for i, row := range rows {
		placeholders := make([]string, len(row))
		for j, value := range row {
			qb.paramCount++
			placeholders[j] = fmt.Sprintf("$%d", qb.paramCount)
			qb.params = append(qb.params, value)
		}
		valueClauses[i] = "(" + strings.Join(placeholders, ", ") + ")"
	}

	query.WriteString(strings.Join(valueClauses, ", "))

	return query.String(), qb.params
}

func (qb *QueryBuilder) BuildUpdate(values map[string]interface{}) (string, []interface{}) {
	var query strings.Builder
	var setClauses []string

	query.WriteString("UPDATE ")
	query.WriteString(qb.table)
	query.WriteString(" SET ")

	for column, value := range values {
		qb.paramCount++
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, qb.paramCount))
		qb.params = append(qb.params, value)
	}

	query.WriteString(strings.Join(setClauses, ", "))

	if len(qb.where) > 0 {
		query.WriteString(" WHERE ")
		qb.buildWhereClause(&query, qb.where)
	}

	return query.String(), qb.params
}

func (qb *QueryBuilder) BuildDelete() (string, []interface{}) {
	var query strings.Builder

	query.WriteString("DELETE FROM ")
	query.WriteString(qb.table)

	if len(qb.where) > 0 {
		query.WriteString(" WHERE ")
		qb.buildWhereClause(&query, qb.where)
	}

	return query.String(), qb.params
}

func (qb *QueryBuilder) BuildCount() (string, []interface{}) {
	var query strings.Builder

	query.WriteString("SELECT COUNT(*) FROM ")
	query.WriteString(qb.table)

	if len(qb.where) > 0 {
		query.WriteString(" WHERE ")
		qb.buildWhereClause(&query, qb.where)
	}

	return query.String(), qb.params
}

func (qb *QueryBuilder) buildWhereClause(query *strings.Builder, clauses []WhereClause) {
	for i, clause := range clauses {
		if i > 0 {
			if clause.Or {
				query.WriteString(" OR ")
			} else {
				query.WriteString(" AND ")
			}
		}

		switch clause.Operator {
		case "IN", "NOT IN":
			values := clause.Value.([]interface{})
			placeholders := make([]string, len(values))
			for j, value := range values {
				qb.paramCount++
				placeholders[j] = fmt.Sprintf("$%d", qb.paramCount)
				qb.params = append(qb.params, value)
			}
			query.WriteString(fmt.Sprintf("%s %s (%s)", clause.Column, clause.Operator, strings.Join(placeholders, ", ")))

		case "BETWEEN":
			values := clause.Value.([]interface{})
			qb.paramCount++
			startParam := qb.paramCount
			qb.params = append(qb.params, values[0])
			qb.paramCount++
			endParam := qb.paramCount
			qb.params = append(qb.params, values[1])
			query.WriteString(fmt.Sprintf("%s BETWEEN $%d AND $%d", clause.Column, startParam, endParam))

		case "IS NULL", "IS NOT NULL":
			query.WriteString(fmt.Sprintf("%s %s", clause.Column, clause.Operator))

		default:
			qb.paramCount++
			query.WriteString(fmt.Sprintf("%s %s $%d", clause.Column, clause.Operator, qb.paramCount))
			qb.params = append(qb.params, clause.Value)
		}
	}
}

func BuildCampaignListQuery(filters *FilterOptions) (string, []interface{}) {
	qb := NewQueryBuilder().
		Table("campaigns").
		Select("id", "name", "status", "total_recipients", "sent_count", "failed_count", "created_at", "updated_at")

	if filters.Status != "" {
		qb.Where("status", "=", filters.Status)
	}

	if filters.Search != "" {
		qb.WhereLike("name", "%"+filters.Search+"%")
	}

	if filters.StartDate != nil {
		qb.Where("created_at", ">=", *filters.StartDate)
	}

	if filters.EndDate != nil {
		qb.Where("created_at", "<=", *filters.EndDate)
	}

	if filters.SortBy != "" {
		qb.OrderBy(filters.SortBy, filters.SortOrder == "desc")
	} else {
		qb.OrderBy("created_at", true)
	}

	if filters.Limit > 0 {
		qb.Limit(filters.Limit)
	}

	if filters.Offset > 0 {
		qb.Offset(filters.Offset)
	}

	return qb.BuildSelect()
}

func BuildAccountListQuery(filters *FilterOptions) (string, []interface{}) {
	qb := NewQueryBuilder().
		Table("accounts").
		Select("id", "email", "provider", "status", "health_score", "daily_sent", "daily_limit", "created_at")

	if filters.Status != "" {
		qb.Where("status", "=", filters.Status)
	}

	if filters.Provider != "" {
		qb.Where("provider", "=", filters.Provider)
	}

	if filters.Search != "" {
		qb.WhereLike("email", "%"+filters.Search+"%")
	}

	if filters.SortBy != "" {
		qb.OrderBy(filters.SortBy, filters.SortOrder == "desc")
	} else {
		qb.OrderBy("created_at", true)
	}

	if filters.Limit > 0 {
		qb.Limit(filters.Limit)
	}

	if filters.Offset > 0 {
		qb.Offset(filters.Offset)
	}

	return qb.BuildSelect()
}

func BuildTemplateListQuery(filters *FilterOptions) (string, []interface{}) {
	qb := NewQueryBuilder().
		Table("templates").
		Select("id", "name", "subject", "spam_score", "version", "created_at", "updated_at")

	if filters.Search != "" {
		qb.Where("name", "LIKE", "%"+filters.Search+"%").
			OrWhere("subject", "LIKE", "%"+filters.Search+"%")
	}

	if filters.SortBy != "" {
		qb.OrderBy(filters.SortBy, filters.SortOrder == "desc")
	} else {
		qb.OrderBy("created_at", true)
	}

	if filters.Limit > 0 {
		qb.Limit(filters.Limit)
	}

	if filters.Offset > 0 {
		qb.Offset(filters.Offset)
	}

	return qb.BuildSelect()
}

func BuildRecipientListQuery(campaignID int64, filters *FilterOptions) (string, []interface{}) {
	qb := NewQueryBuilder().
		Table("recipients").
		Select("id", "email", "name", "status", "sent_at", "created_at")

	qb.Where("campaign_id", "=", campaignID)

	if filters.Status != "" {
		qb.Where("status", "=", filters.Status)
	}

	if filters.Search != "" {
		qb.Where("email", "LIKE", "%"+filters.Search+"%").
			OrWhere("name", "LIKE", "%"+filters.Search+"%")
	}

	if filters.SortBy != "" {
		qb.OrderBy(filters.SortBy, filters.SortOrder == "desc")
	} else {
		qb.OrderBy("created_at", false)
	}

	if filters.Limit > 0 {
		qb.Limit(filters.Limit)
	}

	if filters.Offset > 0 {
		qb.Offset(filters.Offset)
	}

	return qb.BuildSelect()
}

func BuildProxyListQuery(filters *FilterOptions) (string, []interface{}) {
	qb := NewQueryBuilder().
		Table("proxies").
		Select("id", "host", "port", "type", "status", "health_score", "last_checked", "created_at")

	if filters.Status != "" {
		qb.Where("status", "=", filters.Status)
	}

	if filters.Search != "" {
		qb.WhereLike("host", "%"+filters.Search+"%")
	}

	if filters.SortBy != "" {
		qb.OrderBy(filters.SortBy, filters.SortOrder == "desc")
	} else {
		qb.OrderBy("created_at", true)
	}

	if filters.Limit > 0 {
		qb.Limit(filters.Limit)
	}

	if filters.Offset > 0 {
		qb.Offset(filters.Offset)
	}

	return qb.BuildSelect()
}

func BuildLogListQuery(filters *FilterOptions) (string, []interface{}) {
	qb := NewQueryBuilder().
		Table("logs").
		Select("id", "campaign_id", "level", "message", "context", "created_at")

	if filters.Status != "" {
		qb.Where("level", "=", filters.Status)
	}

	if filters.Search != "" {
		qb.WhereLike("message", "%"+filters.Search+"%")
	}

	if filters.StartDate != nil {
		qb.Where("created_at", ">=", *filters.StartDate)
	}

	if filters.EndDate != nil {
		qb.Where("created_at", "<=", *filters.EndDate)
	}

	qb.OrderBy("created_at", true)

	if filters.Limit > 0 {
		qb.Limit(filters.Limit)
	}

	if filters.Offset > 0 {
		qb.Offset(filters.Offset)
	}

	return qb.BuildSelect()
}

func BuildCampaignStatsQuery(campaignID int64) (string, []interface{}) {
	query := `
		SELECT 
			COUNT(*) as total_recipients,
			SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent_count,
			SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_count,
			SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending_count
		FROM recipients
		WHERE campaign_id = $1
	`
	return query, []interface{}{campaignID}
}

func BuildAccountStatsQuery(accountID int64) (string, []interface{}) {
	query := `
		SELECT 
			daily_sent,
			daily_limit,
			total_sent,
			total_failed,
			health_score,
			consecutive_failures
		FROM accounts
		WHERE id = $1
	`
	return query, []interface{}{accountID}
}

func BuildSystemStatsQuery() (string, []interface{}) {
	query := `
		SELECT 
			(SELECT COUNT(*) FROM campaigns WHERE status = 'running') as active_campaigns,
			(SELECT COUNT(*) FROM accounts WHERE status = 'active') as active_accounts,
			(SELECT COUNT(*) FROM recipients WHERE status = 'pending') as pending_emails,
			(SELECT SUM(sent_count) FROM campaigns) as total_sent
	`
	return query, []interface{}{}
}

