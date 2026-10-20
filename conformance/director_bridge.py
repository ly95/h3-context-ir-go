"""Native director configuration/formatting; no lexical request interpretation.

The legacy parse API opts into its historical custom policy. The native result
also records UnknownID/UsedCustom; production Go compilation rejects unknown IDs.
Python identity adaptation preserves the original dataclass `is` assertions only.
"""
import dataclasses


def install(native_operation):
    from h3ir import director as D
    def plain(d):
        return dataclasses.asdict(d) if d is not None else None
    def source(d):
        return D.Director(**d) if d is not None else None
    D.brief_instruction=lambda d,scored=True:native_operation('h3ir.director.brief_instruction',{'director':plain(d),'scored':scored})
    D.note=lambda d:native_operation('h3ir.director.note',{'director':plain(d)})
    D.check=lambda d:native_operation('h3ir.director.check',{'director':plain(d)})
    def from_mapping(data):
        if isinstance(data,D.Director):
            native_operation('h3ir.director.from_mapping',{'director':plain(data)})
            return data
        return source(native_operation('h3ir.director.from_mapping',{'data':data}))
    D.from_mapping=from_mapping
    D.to_mapping=lambda d:native_operation('h3ir.director.to_mapping',{'director':plain(d)})
    D.is_empty=lambda d:native_operation('h3ir.director.is_empty',{'director':plain(d)})
    D.unknown=lambda value:native_operation('h3ir.director.unknown',{'value':value or ''})
    def parse(value,custom=None):
        if isinstance(value,D.Director):
            selection=native_operation('h3ir.director.parse',{'value':'custom','custom':plain(value)})
            return value if selection['used_custom'] else source(selection['director'])
        selection=native_operation('h3ir.director.parse',{'value':value or '', 'custom':plain(custom)})
        result=selection['director']
        if selection['used_custom']:return custom
        if result is None:return None
        return D.BY_ID[result['id']]
    D.parse=parse
