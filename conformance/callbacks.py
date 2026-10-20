"""Synchronous local test callbacks; no callback sends an external message."""
HANDLERS={}

def register(name,handler):
    HANDLERS[name]=handler

def dispatch(name,payload):
    handler=HANDLERS.get(name)
    if handler is None:return {'error':{'kind':'unsupported-test-callback','message':'No test callback is registered'}}
    return handler(payload)
