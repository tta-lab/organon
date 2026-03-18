;; Top-level function definitions
(module
    (function_definition
        name: (identifier) @symbol.name) @symbol.decl)

;; Class definitions
(class_definition
    name: (identifier) @symbol.name) @symbol.decl

;; Class methods (depth 2)
(class_definition
    body: (block
        (function_definition
            name: (identifier) @field.name) @field.decl))
