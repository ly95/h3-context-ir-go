"""Original request/capacity assertions against production native intake APIs."""
import functools
import importlib
import sys


def install(native_operation,plain):
    module=importlib.import_module('h3ir.compile')
    for name in ('check_request','check_capacity'):
        original=getattr(module,name)
        def wrap(name,original):
            @functools.wraps(original)
            def invoke(brief):
                result=native_operation('h3ir.compile.'+name,{'brief':plain(brief)})
                if not result['ok']:
                    if name=='check_capacity':raise module.OverCapacity(result['message'])
                    raise module.BriefRefused(result['code'],result['message'])
            return invoke
        wrapper=wrap(name,original)
        for n,loaded in list(sys.modules.items()):
            if n.startswith('h3ir') and loaded is not None:
                for key,value in list(vars(loaded).items()):
                    if value is original:setattr(loaded,key,wrapper)
        setattr(module,name,wrapper)
