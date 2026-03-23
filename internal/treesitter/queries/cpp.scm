;; Vendored from tree-sitter-cpp tags.scm (doc/predicate blocks stripped)

;; Class definitions
(class_specifier
  name: (type_identifier) @name) @definition.class

;; Struct definitions
(struct_specifier
  name: (type_identifier) @name) @definition.class

;; Enum definitions
(enum_specifier
  name: (type_identifier) @name) @definition.type

;; Typedefs
(type_definition
  declarator: (type_identifier) @name) @definition.type

;; Namespace definitions
(namespace_definition
  name: (identifier) @name) @definition.module

;; Function definitions
(function_definition
  declarator: (function_declarator
    declarator: (identifier) @name)) @definition.function

(function_definition
  declarator: (function_declarator
    declarator: (field_identifier) @name)) @definition.method

;; Function declarations (prototypes)
(declaration
  declarator: (function_declarator
    declarator: (identifier) @name)) @definition.function

;; Template function definitions
(template_declaration
  (function_definition
    declarator: (function_declarator
      declarator: (identifier) @name)) @definition.function)

;; Template class definitions
(template_declaration
  (class_specifier
    name: (type_identifier) @name) @definition.class)
