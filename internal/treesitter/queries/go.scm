;; Top-level function declarations
(function_declaration
    name: (identifier) @symbol.name) @symbol.decl

;; Method declarations
(method_declaration
    receiver: (parameter_list) @symbol.receiver
    name: (field_identifier) @symbol.name) @symbol.decl

;; Type declarations (struct, interface, alias)
(type_declaration
    (type_spec
        name: (type_identifier) @symbol.name)) @symbol.decl

;; Struct fields (depth 2)
(field_declaration
    name: (field_identifier) @field.name) @field.decl

;; Package-level vars and consts
(var_declaration) @symbol.decl
(const_declaration) @symbol.decl
