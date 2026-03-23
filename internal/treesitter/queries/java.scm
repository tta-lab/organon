;; Vendored from tree-sitter-java tags.scm

;; Class declarations
(class_declaration
  name: (identifier) @name) @definition.class

;; Interface declarations
(interface_declaration
  name: (identifier) @name) @definition.interface

;; Enum declarations
(enum_declaration
  name: (identifier) @name) @definition.type

;; Method declarations
(method_declaration
  name: (identifier) @name) @definition.method

;; Constructor declarations
(constructor_declaration
  name: (identifier) @name) @definition.constructor

;; References
(method_invocation
  name: (identifier) @name) @reference.call

(object_creation_expression
  type: (type_identifier) @name) @reference.class
