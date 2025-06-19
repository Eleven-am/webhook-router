# JavaScript Transformation with Validation

There are two approaches to validate data when using JavaScript transformations:

## Approach 1: Built-in JavaScript Validation

You can implement validation logic directly in your JavaScript transformation:

```json
{
  "type": "js_transform",
  "config": {
    "script": "
      function transform(input) {
        // Validation logic
        if (!input.email) {
          throw new Error('Email is required');
        }
        
        if (!input.email.match(/^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/)) {
          throw new Error('Invalid email format');
        }
        
        if (input.age && (input.age < 0 || input.age > 150)) {
          throw new Error('Age must be between 0 and 150');
        }
        
        // Transformation logic
        return {
          ...input,
          email: input.email.toLowerCase(),
          validated: true,
          validated_at: formatDate(now(), 'iso')
        };
      }
    ",
    "error_handling": "stop"  // Stop pipeline on validation error
  }
}
```

## Approach 2: Separate Validation Stage

Use the dedicated validation stage after JavaScript transformation:

```json
{
  "stages": [
    {
      "name": "transform_user_data",
      "type": "js_transform",
      "config": {
        "script": "
          function transform(input) {
            return {
              id: input.user_id || input.id,
              email: input.email ? input.email.toLowerCase() : '',
              name: (input.first_name + ' ' + input.last_name).trim(),
              age: parseInt(input.age) || null,
              created_at: formatDate(now(), 'iso')
            };
          }
        "
      }
    },
    {
      "name": "validate_user_data",
      "type": "validate",
      "config": {
        "fields": {
          "id": "required,numeric",
          "email": "required,email",
          "name": "required,min=2,max=100",
          "age": "omitempty,numeric,min=0,max=150"
        },
        "on_failure": "stop",
        "error_messages": {
          "email": "A valid email address is required",
          "name": "Name must be between 2 and 100 characters"
        }
      }
    }
  ]
}
```

## Approach 3: Hybrid - Transform with Validation Helper

Create a reusable validation function in JavaScript:

```json
{
  "type": "js_transform",
  "config": {
    "script": "
      // Validation helper
      function validate(data, rules) {
        const errors = [];
        
        for (const field in rules) {
          const value = data[field];
          const fieldRules = rules[field].split(',');
          
          for (const rule of fieldRules) {
            if (rule === 'required' && !value) {
              errors.push(field + ' is required');
            }
            else if (rule === 'email' && value && !value.match(/^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/)) {
              errors.push(field + ' must be a valid email');
            }
            else if (rule.startsWith('min=')) {
              const min = parseInt(rule.split('=')[1]);
              if (value && value.length < min) {
                errors.push(field + ' must be at least ' + min + ' characters');
              }
            }
            else if (rule.startsWith('max=')) {
              const max = parseInt(rule.split('=')[1]);
              if (value && value.length > max) {
                errors.push(field + ' must be at most ' + max + ' characters');
              }
            }
          }
        }
        
        return errors;
      }
      
      // Main transformation
      function transform(input) {
        // Define validation rules
        const rules = {
          email: 'required,email',
          name: 'required,min=2,max=100',
          age: 'min=0,max=150'
        };
        
        // Validate
        const errors = validate(input, rules);
        if (errors.length > 0) {
          return {
            success: false,
            errors: errors,
            original_data: input
          };
        }
        
        // Transform if valid
        return {
          success: true,
          data: {
            email: input.email.toLowerCase(),
            name: input.name.trim(),
            age: parseInt(input.age) || null,
            processed_at: now()
          }
        };
      }
    ",
    "error_handling": "continue"  // Let the pipeline handle the validation result
  }
}
```

## Complex Example: Order Processing with Validation

```json
{
  "type": "js_transform",
  "config": {
    "script": "
      function transform(input) {
        // Validate order structure
        if (!input.order_id) {
          throw new Error('Order ID is required');
        }
        
        if (!input.items || !Array.isArray(input.items) || input.items.length === 0) {
          throw new Error('Order must contain at least one item');
        }
        
        // Validate and transform items
        const processedItems = input.items.map((item, index) => {
          // Validate item
          if (!item.product_id) {
            throw new Error('Item ' + (index + 1) + ': Product ID is required');
          }
          
          if (!item.quantity || item.quantity <= 0) {
            throw new Error('Item ' + (index + 1) + ': Quantity must be greater than 0');
          }
          
          if (!item.price || item.price < 0) {
            throw new Error('Item ' + (index + 1) + ': Price must be non-negative');
          }
          
          // Calculate item total
          return {
            ...item,
            total: item.quantity * item.price,
            validated: true
          };
        });
        
        // Calculate order totals
        const subtotal = processedItems.reduce((sum, item) => sum + item.total, 0);
        const tax = subtotal * 0.1; // 10% tax
        const total = subtotal + tax;
        
        // Validate totals
        if (total > 10000) {
          throw new Error('Order total exceeds maximum allowed amount');
        }
        
        return {
          order_id: input.order_id,
          customer_id: input.customer_id,
          items: processedItems,
          subtotal: Math.round(subtotal * 100) / 100,
          tax: Math.round(tax * 100) / 100,
          total: Math.round(total * 100) / 100,
          validated_at: formatDate(now(), 'iso'),
          validation_version: '1.0'
        };
      }
    ",
    "error_handling": "stop",
    "timeout_ms": 3000
  }
}
```

## Best Practices

### When to Use Built-in JavaScript Validation:
- Complex business logic validation
- Cross-field validation
- Dynamic validation rules
- Custom error formatting
- Validation that depends on transformation logic

### When to Use Separate Validation Stage:
- Standard field validation (email, required, min/max)
- Reusable validation rules across pipelines
- When you want clear separation of concerns
- When using standard validation libraries

### Performance Considerations:
1. **Built-in validation** is faster (single stage execution)
2. **Separate validation** is more maintainable and reusable
3. **Hybrid approach** offers flexibility with some performance overhead

## Error Handling Strategies

### Stop on Error (Recommended for critical validation):
```json
{
  "error_handling": "stop"
}
```

### Continue and Let Next Stage Handle:
```json
{
  "error_handling": "continue"
}
```

### Transform Errors into Data:
```json
{
  "script": "
    try {
      // validation and transformation
      return {success: true, data: transformedData};
    } catch (e) {
      return {success: false, error: e.message, original: input};
    }
  ",
  "error_handling": "continue"
}
```

## Testing Validation

You can test your validation logic before deploying:

```javascript
// Test data
const testCases = [
  {email: "test@example.com", name: "John Doe", age: 25},  // Valid
  {email: "invalid-email", name: "John", age: 200},        // Invalid email and age
  {name: "Jane", age: 30},                                 // Missing email
  {email: "test@test.com"},                                // Missing name
];

// Run tests in your transformation
function transform(input) {
  if (input._test) {
    return testCases.map(test => {
      try {
        const result = validateAndTransform(test);
        return {input: test, result: result, status: 'passed'};
      } catch (e) {
        return {input: test, error: e.message, status: 'failed'};
      }
    });
  }
  
  // Normal processing
  return validateAndTransform(input);
}
```