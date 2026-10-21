"""Native formatting/algebra from already interpreted structured scope only.

No request text is examined. Semantic build/intent resolution is the separate
scope_bridge. The source dataclass is transport; no product formatter runs here.
"""
import dataclasses


def install(native_operation):
    from h3ir import creativity as C

    def invoke(name, value, **kwargs):
        return native_operation('h3ir.creativity.Scope.' + name,
                                {'self': dataclasses.asdict(value), **kwargs})

    C.Scope.note = lambda self: invoke('note', self)
    C.Scope.brief_instruction = lambda self: invoke('brief_instruction', self)
    C.Scope.why_not = lambda self, element: invoke('why_not', self, element=element)
    C.Scope.permits = lambda self, element: invoke('permits', self, element=element)
    C.Scope.magnitude = property(lambda self: invoke('magnitude', self))
    C.Scope.licensed = property(lambda self: frozenset(invoke('licensed', self)))
