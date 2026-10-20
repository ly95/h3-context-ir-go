"""CI-only transport guard. Original test bodies and assertions stay untouched."""
import ipaddress
import socket


def _loopback(host):
    if isinstance(host, bytes):
        host = host.decode("ascii")
    if host in (None, "", "localhost"):
        return True
    try:
        return ipaddress.ip_address(str(host).split("%", 1)[0]).is_loopback
    except ValueError:
        return False


def pytest_configure(config):
    # The CI job additionally has a Linux network namespace with only loopback;
    # this guard gives an actionable failure before an accidental DNS lookup.
    original_lookup = socket.getaddrinfo
    original_connect = socket.socket.connect
    original_connect_ex = socket.socket.connect_ex

    def lookup(host, *args, **kwargs):
        if not _loopback(host):
            raise RuntimeError("offline conformance forbids non-loopback DNS/network access")
        return original_lookup(host, *args, **kwargs)

    def guarded_connect(original):
        def connect(sock, address):
            if sock.family in (socket.AF_INET, socket.AF_INET6) and not _loopback(address[0]):
                raise RuntimeError("offline conformance forbids non-loopback network access")
            return original(sock, address)
        return connect

    socket.getaddrinfo = lookup
    socket.socket.connect = guarded_connect(original_connect)
    socket.socket.connect_ex = guarded_connect(original_connect_ex)
