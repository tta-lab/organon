;; Vendored from tree-sitter-c tags.scm

;; Function definitions
(function_definition
  declarator: (function_declarator
    declarator: (identifier) @name)) @definition.function

;; Function declarations (prototypes)
(declaration
  declarator: (function_declarator
    declarator: (identifier) @name)) @definition.function

;; Struct definitions
(struct_specifier
  name: (type_identifier) @name) @definition.class

;; Enum definitions
(enum_specifier
  name: (type_identifier) @name) @definition.type

;; Typedef declarations
(type_definition
  declarator: (type_identifier) @name) @definition.type

;; References
(call_expression
  function: (identifier) @name) @reference.call
