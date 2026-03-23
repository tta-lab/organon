;; Vendored from tree-sitter-rust tags.scm
;; impl_item promoted from @reference to @definition (organon extension)

;; ADT definitions
(struct_item
    name: (type_identifier) @name) @definition.class

(enum_item
    name: (type_identifier) @name) @definition.class

(union_item
    name: (type_identifier) @name) @definition.class

;; type aliases
(type_item
    name: (type_identifier) @name) @definition.class

;; method definitions (inside impl/trait blocks)
(declaration_list
    (function_item
        name: (identifier) @name) @definition.method)

;; function definitions
(function_item
    name: (identifier) @name) @definition.function

;; trait definitions
(trait_item
    name: (type_identifier) @name) @definition.interface

;; module definitions
(mod_item
    name: (identifier) @name) @definition.module

;; macro definitions
(macro_definition
    name: (identifier) @name) @definition.macro

;; impl blocks (upstream has as @reference.implementation — promoted to definition)
(impl_item
    type: (type_identifier) @name) @definition.impl

;; references
(call_expression
    function: (identifier) @name) @reference.call

(call_expression
    function: (field_expression
        field: (field_identifier) @name)) @reference.call

(macro_invocation
    macro: (identifier) @name) @reference.call
