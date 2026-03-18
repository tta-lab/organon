;; Function declarations
(function_declaration
    name: (identifier) @symbol.name) @symbol.decl

;; Class declarations
(class_declaration
    name: (type_identifier) @symbol.name) @symbol.decl

;; Interface declarations
(interface_declaration
    name: (type_identifier) @symbol.name) @symbol.decl

;; Type alias declarations
(type_alias_declaration
    name: (type_identifier) @symbol.name) @symbol.decl

;; Class members (depth 2)
(method_definition
    name: (property_identifier) @field.name) @field.decl

(public_field_definition
    name: (property_identifier) @field.name) @field.decl
