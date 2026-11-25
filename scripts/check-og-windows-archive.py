#!/usr/bin/env python3
"""Verify the snapshot archives and the Windows x64 og executable."""

import hashlib
import pathlib
import struct
import sys
import tarfile
import zipfile

dist = pathlib.Path(sys.argv[1])
checksums = {}
for line in (dist / "checksums.txt").read_text().splitlines():
    digest, name = line.split(maxsplit=1)
    name = name.lstrip("*")
    path = dist / name
    if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
        raise SystemExit(f"checksum mismatch: {name}")
    checksums[name] = digest

windows = list(dist.glob("organon_Windows_x86_64_og.zip"))
if len(windows) != 1 or windows[0].name not in checksums:
    raise SystemExit("missing checksummed Windows og archive")
with zipfile.ZipFile(windows[0]) as archive:
    executable = archive.read("og.exe")
    offset = struct.unpack_from("<I", executable, 0x3C)[0]
    if executable[:2] != b"MZ" or executable[offset : offset + 6] != b"PE\0\0\x64\x86":
        raise SystemExit("og.exe is not a Windows x64 PE executable")

for platform in ("Linux_x86_64", "Linux_arm64", "Darwin_x86_64", "Darwin_arm64"):
    name = f"organon_{platform}.tar.gz"
    if name not in checksums:
        raise SystemExit(f"missing checksummed archive: {name}")
    with tarfile.open(dist / name) as archive:
        members = {pathlib.PurePosixPath(member.name).name for member in archive.getmembers()}
        if not {"src", "skill", "goal", "token", "og"} <= members:
            raise SystemExit(f"missing binaries in {name}")

print("Windows x64 og ZIP, existing archives, and checksums verified")
