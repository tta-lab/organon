;; Vendored from tree-sitter-go tags.scm (doc/predicate blocks stripped)
;; Note: type_declaration used instead of type_spec so StartByte points to
;; the `type` keyword — required for correct comment-write and symbol range.

(function_declaration
  name: (identifier) @name) @definition.function

(method_declaration
  name: (field_identifier) @name) @definition.method

(call_expression
  function: [
    (identifier) @name
    (parenthesized_expression (identifier) @name)
    (selector_expression field: (field_identifier) @name)
    (parenthesized_expression (selector_expression field: (field_identifier) @name))
  ]) @reference.call

(type_declaration
  (type_spec name: (type_identifier) @name)) @definition.type
