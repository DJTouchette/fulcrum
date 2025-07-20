# Model Validation System 🚧

## Status: TODO - HIGH PRIORITY

## Description
Runtime validation system that enforces the validation rules defined in YAML model configurations on both Go and TypeScript sides.

## Current State
- YAML validation rules are parsed and stored
- Validation helper methods exist (`IsNullable()`, `GetLengthConstraints()`)
- No actual runtime validation enforcement
- TypeScript side has no validation integration

## Required Implementation
- [ ] Go-side validation before database operations
- [ ] TypeScript-side validation in domain handlers
- [ ] Validation error messages and formatting
- [ ] Custom validation rule system
- [ ] Integration with HTTP error responses
- [ ] Validation rule documentation generation

## Technical Requirements

### Go-Side Validation
```go
type Validator struct {
    appConfig *parser.AppConfig
}

func (v *Validator) ValidateModel(domainName, modelName string, data map[string]any) error
func (v *Validator) ValidateField(field parser.Field, value any) error
```

### Validation Rules to Implement
Based on current YAML parsing:
```yaml
models:
  - user:
      email:
        type: text
        validations:
          - nullable: false
          - length: {min: 5, max: 80}
      age:
        type: integer
        validations:
          - range: {min: 0, max: 120}
```

### Built-in Validation Types
- `nullable: false` - Field cannot be null/empty
- `length: {min: 5, max: 80}` - String length constraints
- `range: {min: 0, max: 120}` - Numeric range validation
- `pattern: "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$"` - Regex validation
- `unique: true` - Database uniqueness constraint
- `foreign_key: "users.id"` - Foreign key validation

### TypeScript Integration
Add validation to the TypeScript domain framework:
```javascript
class DomainBase {
    async validateData(modelName, data) {
        // Call back to Go for validation via gRPC
        const result = await this.client.sendMessage('validate_model', {
            model: modelName,
            data: data
        }, true);
        
        if (!result.success) {
            throw new ValidationError(result.errors);
        }
    }
}
```

### Error Response Format
```json
{
  "success": false,
  "error": "Validation failed",
  "validation_errors": [
    {
      "field": "email",
      "message": "Email must be between 5 and 80 characters",
      "code": "length_constraint"
    },
    {
      "field": "name", 
      "message": "Name is required",
      "code": "required_field"
    }
  ]
}
```

## Integration Points
- Integrate with database operations in `grpc.go`
- Add validation gRPC message types
- Connect to TypeScript domain methods
- Integrate with HTTP error responses
- Fix typo in `syntax.go` (ValidateLengthMax = "maz" → "max")

## Implementation Strategy
1. **Phase 1**: Basic validation
   - Implement nullable and length validations
   - Add Go-side validation before database operations
   - Create validation error types and responses
2. **Phase 2**: TypeScript integration
   - Add gRPC validation message type
   - Integrate validation into domain handlers
   - Add validation helper methods to DomainBase
3. **Phase 3**: Advanced validations
   - Range, pattern, and custom validations
   - Database uniqueness validation
   - Foreign key constraint validation

## gRPC Message Types
Add new message types to `framework.proto`:
- `validate_model_request` - Validate data against model
- `validate_model_response` - Return validation results

## CLI Integration
Potential commands:
```bash
fulcrum validate config     # Validate YAML configurations
fulcrum generate schema     # Generate validation schema docs
```

## Dependencies
- Regular expression library for pattern validation
- Integration with database system for uniqueness/foreign keys

## Success Criteria
- [ ] All YAML validation rules are enforced at runtime
- [ ] Both Go and TypeScript sides validate data consistently
- [ ] Clear validation error messages returned to users
- [ ] Database operations rejected for invalid data
- [ ] HTTP responses include validation error details
- [ ] Performance impact is minimal

## Estimated Effort
**Medium** (5-7 days) - Well-scoped but requires coordination between Go and TypeScript sides.

## Related Features
- Links to Database Integration (database-level constraints)
- Links to HTTP Server (error response formatting)
- Links to Config Parser (uses parsed validation rules)

## Notes
This is essential for data integrity and user experience. Without validation, the framework allows invalid data to reach the database.
