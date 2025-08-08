package database

import (
	"context"
	"encoding/json"
	"fmt"
	"fulcrum/lib/database/interfaces"
	"reflect"
	"strconv"
	"strings"
)

// DatabaseExecutor handles JSON to SQL conversion and back
type DatabaseExecutor struct {
	db interfaces.Database
}

func NewDatabaseExecutor(db interfaces.Database) *DatabaseExecutor {
	return &DatabaseExecutor{db: db}
}

// SingleOperationRequest represents a direct method call (create, update, find)
type SingleOperationRequest struct {
	Operation string         `json:"operation"` // "create", "update", "find"
	Table     string         `json:"table"`
	ID        any            `json:"id,omitempty"`    // for update
	Data      map[string]any `json:"data,omitempty"`  // for create/update
	Query     map[string]any `json:"query,omitempty"` // for find
	RequestID *string        `json:"request_id,omitempty"`
}

// OperationResponse represents the response
type OperationResponse struct {
	Success   bool             `json:"success"`
	Data      []map[string]any `json:"data,omitempty"`
	Error     string           `json:"error,omitempty"`
	Count     int              `json:"count"`
	RequestID *string          `json:"request_id,omitempty"`
}

// CreateRecord handles direct create calls
func (de *DatabaseExecutor) CreateRecord(ctx context.Context, table string, data map[string]any, requestID *string) ([]byte, error) {
	req := SingleOperationRequest{
		Operation: "create",
		Table:     table,
		Data:      data,
		RequestID: requestID,
	}
	return de.executeOperation(ctx, req)
}

// UpdateRecord handles direct update calls
func (de *DatabaseExecutor) UpdateRecord(ctx context.Context, table string, id any, data map[string]any, requestID *string) ([]byte, error) {
	req := SingleOperationRequest{
		Operation: "update",
		Table:     table,
		ID:        id,
		Data:      data,
		RequestID: requestID,
	}
	return de.executeOperation(ctx, req)
}

// FindRecords handles direct find calls
func (de *DatabaseExecutor) FindRecords(ctx context.Context, table string, query map[string]any, requestID *string) ([]byte, error) {
	if query == nil {
		query = make(map[string]any)
	}

	req := SingleOperationRequest{
		Operation: "find",
		Table:     table,
		Query:     query,
		RequestID: requestID,
	}
	return de.executeOperation(ctx, req)
}

// ExecuteJSON is a generic handler that can accept JSON from any source
func (de *DatabaseExecutor) ExecuteJSON(ctx context.Context, requestJSON []byte) ([]byte, error) {
	var req SingleOperationRequest
	if err := json.Unmarshal(requestJSON, &req); err != nil {
		return de.errorResponse("Invalid JSON request: "+err.Error(), req.RequestID)
	}
	return de.executeOperation(ctx, req)
}

// executeOperation handles the actual database operation
func (de *DatabaseExecutor) executeOperation(ctx context.Context, req SingleOperationRequest) ([]byte, error) {
	var response OperationResponse
	response.RequestID = req.RequestID

	switch req.Operation {
	case "create":
		response = de.createRecord(ctx, req.Table, req.Data)
	case "update":
		response = de.updateRecord(ctx, req.Table, req.ID, req.Data)
	case "find":
		response = de.findRecords(ctx, req.Table, req.Query)
	default:
		response = OperationResponse{
			Success: false,
			Error:   "Unsupported operation: " + req.Operation,
		}
	}

	response.RequestID = req.RequestID
	return json.Marshal(response)
}

// createRecord handles INSERT operations
func (de *DatabaseExecutor) createRecord(ctx context.Context, table string, data map[string]any) OperationResponse {
	if len(data) == 0 {
		return OperationResponse{
			Success: false,
			Error:   "No data provided for create",
		}
	}

	fields := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	args := make([]any, 0, len(data))

	for field, value := range data {
		fields = append(fields, field)
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(fields, ", "),
		strings.Join(placeholders, ", "))

	result, err := de.db.Exec(ctx, query, args...)
	if err != nil {
		return OperationResponse{
			Success: false,
			Error:   "Create failed: " + err.Error(),
		}
	}

	affected, _ := result.RowsAffected()
	response := OperationResponse{
		Success: true,
		Count:   int(affected),
	}

	// Include the inserted record data with ID if available
	recordData := make(map[string]any)
	for k, v := range data {
		recordData[k] = v
	}

	if id, err := result.LastInsertId(); err == nil {
		recordData["id"] = id
	}

	response.Data = []map[string]any{recordData}
	return response
}

// updateRecord handles UPDATE operations
func (de *DatabaseExecutor) updateRecord(ctx context.Context, table string, id any, data map[string]any) OperationResponse {
	if len(data) == 0 {
		return OperationResponse{
			Success: false,
			Error:   "No data provided for update",
		}
	}

	setParts := make([]string, 0, len(data))
	args := make([]any, 0, len(data)+1)

	for field, value := range data {
		setParts = append(setParts, field+" = ?")
		args = append(args, value)
	}

	// Add ID to args
	args = append(args, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = ?",
		table,
		strings.Join(setParts, ", "))

	result, err := de.db.Exec(ctx, query, args...)
	if err != nil {
		return OperationResponse{
			Success: false,
			Error:   "Update failed: " + err.Error(),
		}
	}

	affected, _ := result.RowsAffected()

	// Return the updated record data
	recordData := make(map[string]any)
	for k, v := range data {
		recordData[k] = v
	}
	recordData["id"] = id

	return OperationResponse{
		Success: true,
		Count:   int(affected),
		Data:    []map[string]any{recordData},
	}
}

// findRecords handles SELECT operations
func (de *DatabaseExecutor) findRecords(ctx context.Context, table string, query map[string]any) OperationResponse {
	var sqlQuery strings.Builder
	var args []any

	sqlQuery.WriteString("SELECT * FROM " + table)

	// Handle query conditions
	if len(query) > 0 {
		// Create a copy to avoid modifying the original
		queryConditions := make(map[string]any)
		for k, v := range query {
			queryConditions[k] = v
		}

		// Handle special query parameters first
		if limit, exists := queryConditions["_limit"]; exists {
			delete(queryConditions, "_limit")
			if limitInt, ok := de.toInt(limit); ok {
				defer func() {
					sqlQuery.WriteString(fmt.Sprintf(" LIMIT %d", limitInt))
				}()
			}
		}

		if offset, exists := queryConditions["_offset"]; exists {
			delete(queryConditions, "_offset")
			if offsetInt, ok := de.toInt(offset); ok {
				defer func() {
					sqlQuery.WriteString(fmt.Sprintf(" OFFSET %d", offsetInt))
				}()
			}
		}

		if orderBy, exists := queryConditions["_order"]; exists {
			delete(queryConditions, "_order")
			if orderStr, ok := orderBy.(string); ok {
				defer func() {
					sqlQuery.WriteString(" ORDER BY " + orderStr)
				}()
			}
		}

		// Build WHERE clause from remaining conditions
		if len(queryConditions) > 0 {
			whereClause, whereArgs := de.buildWhereClause(queryConditions)
			if whereClause != "" {
				sqlQuery.WriteString(" WHERE " + whereClause)
				args = append(args, whereArgs...)
			}
		}
	}

	fmt.Println("HEERE =============================================")
	fmt.Println("Executing SQL Query:", sqlQuery.String(), "Args:", args)
	fmt.Println("HEERE =============================================")

	rows, err := de.db.Query(ctx, sqlQuery.String(), args...)
	if err != nil {
		fmt.Printf("❌ DB Query Error: %v\n", err)
		return OperationResponse{
			Success: false,
			Error:   "Find failed: " + err.Error(),
		}
	}
	fmt.Println("✅ DB Query executed successfully")
	defer rows.Close()

	data, err := de.rowsToJSON(rows)
	if err != nil {
		fmt.Printf("❌ rowsToJSON Error: %v\n", err)
		return OperationResponse{
			Success: false,
			Error:   "Failed to convert results: " + err.Error(),
		}
	}

	fmt.Printf("✅ rowsToJSON successful - Records found: %d\n", len(data))
	fmt.Printf("📊 Data preview: %+v\n", data)

	return OperationResponse{
		Success: true,
		Data:    data,
		Count:   len(data),
	}
}

// buildWhereClause creates WHERE conditions from JSON
func (de *DatabaseExecutor) buildWhereClause(where map[string]any) (string, []any) {
	var conditions []string
	var args []any
	paramIndex := 1 // PostgreSQL parameters start at $1

	for field, value := range where {
		// Skip special parameters that start with underscore
		if strings.HasPrefix(field, "_") {
			continue
		}
		// Handle different comparison operators
		if strings.Contains(field, "__") {
			parts := strings.Split(field, "__")
			field = parts[0]
			op := parts[1]
			switch op {
			case "gt":
				conditions = append(conditions, fmt.Sprintf("%s > $%d", field, paramIndex))
				args = append(args, value)
				paramIndex++
			case "gte":
				conditions = append(conditions, fmt.Sprintf("%s >= $%d", field, paramIndex))
				args = append(args, value)
				paramIndex++
			case "lt":
				conditions = append(conditions, fmt.Sprintf("%s < $%d", field, paramIndex))
				args = append(args, value)
				paramIndex++
			case "lte":
				conditions = append(conditions, fmt.Sprintf("%s <= $%d", field, paramIndex))
				args = append(args, value)
				paramIndex++
			case "like":
				conditions = append(conditions, fmt.Sprintf("%s LIKE $%d", field, paramIndex))
				args = append(args, value)
				paramIndex++
			case "in":
				// Handle IN clause for arrays
				if arr, ok := value.([]any); ok {
					var placeholders []string
					for i := 0; i < len(arr); i++ {
						placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex))
						paramIndex++
					}
					conditions = append(conditions, fmt.Sprintf("%s IN (%s)", field, strings.Join(placeholders, ",")))
					args = append(args, arr...)
				}
			default:
				conditions = append(conditions, fmt.Sprintf("%s = $%d", field, paramIndex))
				args = append(args, value)
				paramIndex++
			}
		} else {
			conditions = append(conditions, fmt.Sprintf("%s = $%d", field, paramIndex))
			args = append(args, value)
			paramIndex++
		}
	}
	return strings.Join(conditions, " AND "), args
}

// rowsToJSON converts database rows to JSON-friendly format
func (de *DatabaseExecutor) rowsToJSON(rows interfaces.Rows) ([]map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		valuePointers := make([]any, len(columns))

		for i := range values {
			valuePointers[i] = &values[i]
		}

		if err := rows.Scan(valuePointers...); err != nil {
			return nil, err
		}

		row := make(map[string]any)
		for i, column := range columns {
			row[column] = de.normalizeValue(values[i])
		}

		results = append(results, row)
	}

	return results, nil
}

// normalizeValue converts database values to JSON-friendly types
func (de *DatabaseExecutor) normalizeValue(value any) any {
	if value == nil {
		return nil
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			// Convert []byte to string
			return string(value.([]byte))
		}
	}

	return value
}

// Helper function to convert interface{} to int
func (de *DatabaseExecutor) toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i, true
		}
	}
	return 0, false
}

// errorResponse creates a standardized error response
func (de *DatabaseExecutor) errorResponse(message string, requestID *string) ([]byte, error) {
	response := OperationResponse{
		Success:   false,
		Error:     message,
		RequestID: requestID,
	}
	return json.Marshal(response)
}
