;; Function items
(function_item
    name: (identifier) @symbol.name) @symbol.decl

;; Struct items
(struct_item
    name: (type_identifier) @symbol.name) @symbol.decl

;; Enum items
(enum_item
    name: (type_identifier) @symbol.name) @symbol.decl

;; Impl blocks
(impl_item
    type: (type_identifier) @symbol.name) @symbol.decl

;; Trait items
(trait_item
    name: (type_identifier) @symbol.name) @symbol.decl

;; Struct fields (depth 2)
(field_declaration
    name: (field_identifier) @field.name) @field.decl
