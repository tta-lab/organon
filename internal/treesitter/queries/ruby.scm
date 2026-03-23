;; Vendored from tree-sitter-ruby tags.scm (doc/predicate blocks stripped)

;; Method definitions
(method
  name: [(identifier) (operator)] @name) @definition.method

(singleton_method
  name: [(identifier) (operator)] @name) @definition.method

;; Class definitions
(class
  name: [(constant) (scope_resolution)] @name) @definition.class

(singleton_class
  value: [(self) (identifier)] @name) @definition.class

;; Module definitions
(module
  name: [(constant) (scope_resolution)] @name) @definition.module

;; References
(call
  method: (identifier) @name) @reference.call
