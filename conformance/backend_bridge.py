"""Run original backend assertions through Go and their unchanged MockTransport.

The loopback server only carries HTTP bytes between Go and the original pytest
handler. Go owns endpoint paths, headers, payloads, discovery, retry and parsing.
No production endpoint or credential is discovered by this test adapter.
"""
import base64
import contextlib
import functools
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib
import inspect
import threading
import sys


def install(native_operation, plain):
    backend = importlib.import_module("h3ir.backend")
    original_http = backend.Backend._http

    @contextlib.contextmanager
    def proxy(instance):
        client = original_http(instance)
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass  # Test transport never logs headers, payloads or credentials.
            def do_GET(self):
                self.forward()
            def do_POST(self):
                self.forward()
            def forward(self):
                size = int(self.headers.get("Content-Length", "0"))
                body = self.rfile.read(size)
                target = "http://" + self.headers["Host"] + self.path
                try:
                    response = client.request(self.command, target, content=body,
                                              headers=dict(self.headers))
                    self.send_response(response.status_code)
                    self.send_header("Content-Length", str(len(response.content)))
                    self.end_headers(); self.wfile.write(response.content)
                except Exception as error:
                    import httpx
                    kind = ("ConnectTimeout" if isinstance(error, httpx.ConnectTimeout) else
                            "ConnectError" if isinstance(error, httpx.ConnectError) else "HTTPError")
                    self.send_response(599)
                    self.send_header("X-Conformance-Transport-Error", kind)
                    self.send_header("Content-Length", "0")
                    self.end_headers()
        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=lambda: server.serve_forever(poll_interval=0.005), daemon=True)
        thread.start()
        try:
            yield "http://127.0.0.1:" + str(server.server_port)
        finally:
            server.shutdown(); server.server_close(); thread.join()

    def configure(instance):
        c = instance.cfg
        return {"base_url": c.base_url, "api_key": c.api_key, "model": c.model,
                "resolved_model": instance._resolved_model or "", "max_tokens": c.max_tokens,
                "timeout_s": c.timeout_s, "default_thinking": c.default_thinking,
                "guided_decoding": c.guided_decoding}

    def call(name, arguments, instance=None):
        if instance is None:
            result = native_operation("h3ir.backend." + name, arguments)
        else:
            arguments = {**arguments, "config": configure(instance)}
            pure = name in {"Backend.base_url", "Backend._root_url", "Backend.model_id",
                            "Backend._headers", "Backend.liveness_urls"}
            if pure:
                result = native_operation("h3ir.backend." + name,
                                          {**arguments, "proxy_url": "http://127.0.0.1:1"})
            else:
                with proxy(instance) as endpoint:
                    result = native_operation("h3ir.backend." + name,
                                              {**arguments, "proxy_url": endpoint})
            if result.get("model") and not instance.cfg.model:
                instance._resolved_model = result["model"]
        if result.get("error"):
            error = result["error"]
            if error["code"] == "endpoint_refused":
                raise backend.EndpointRefused(error["status"], error["server_message"])
            cls = (backend.TruncatedResponse if error["code"] == "truncated" else
                   backend.BackendUnavailable if error["code"] == "unavailable" else backend.BackendError)
            raise cls(error["message"])
        value = result.get("result")
        if name == "Backend.health_probe":
            return backend.HealthProbe(**{**value, "attempts": tuple(tuple(x) for x in value["attempts"])})
        if name == "Backend.chat":
            keys = backend.Reply.__dataclass_fields__
            return backend.Reply(**{k: v for k, v in value.items() if k in keys})
        if name == "Backend.liveness_urls":
            return tuple(value)
        if name == "vision_check":
            return tuple(value)
        if name == "digits_png":
            return base64.b64decode(value)
        return value

    def replace(owner, name, qualified, instance_argument=False):
        original = getattr(owner, name); signature = inspect.signature(original)
        @functools.wraps(original)
        def wrapped(*args, **kwargs):
            bound = signature.bind(*args, **kwargs); bound.apply_defaults()
            arguments = dict(bound.arguments)
            instance = arguments.pop("self", None) if instance_argument else None
            return call(qualified, arguments, instance)
        for module_name, loaded in list(sys.modules.items()):
            if module_name.startswith("h3ir") and loaded is not None:
                for key, value in list(vars(loaded).items()):
                    if value is original:
                        setattr(loaded, key, wrapped)
        setattr(owner, name, wrapped)

    for name in ("base_url", "_root_url", "model_id", "_headers", "liveness_urls",
                 "health_probe", "health", "_discover_model", "require_available",
                 "server_version", "chat", "json_call"):
        replace(backend.Backend, name, "Backend." + name, True)
    for name in ("_server_sentence", "_extract_json", "_append_text", "_is_the_schema_itself",
                 "image_data_url", "user_message", "digits_png", "judges_the_picture"):
        replace(backend, name, name)
    original_vision = backend.vision_check
    @functools.wraps(original_vision)
    def vision_check(b):
        return call("vision_check", {}, b)
    original_probe = backend.probe
    @functools.wraps(original_probe)
    def probe(cfg=None):
        c = cfg or backend.get_config()
        with backend.Backend(c) as instance:
            return call("probe", {}, instance)
    for name, original, wrapped in (("vision_check", original_vision, vision_check),
                                    ("probe", original_probe, probe)):
        for module_name, loaded in list(sys.modules.items()):
            if module_name.startswith("h3ir") and loaded is not None:
                for key, value in list(vars(loaded).items()):
                    if value is original:
                        setattr(loaded, key, wrapped)
        setattr(backend, name, wrapped)
