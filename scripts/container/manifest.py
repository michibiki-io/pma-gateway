#!/usr/bin/env python3
"""Validate the two native release digests and their combined image index."""
import argparse
import json
from pathlib import Path
import re
import sys

ARCHES = ("amd64", "arm64")
DIGEST = re.compile(r"sha256:[0-9a-f]{64}")
INDEX_TYPES = {
    "application/vnd.oci.image.index.v1+json",
    "application/vnd.docker.distribution.manifest.list.v2+json",
}


def digest(value):
    if not isinstance(value, str) or not DIGEST.fullmatch(value):
        raise ValueError(f"invalid image digest: {value!r}")
    return value


def record(arch, value, directory):
    if arch not in ARCHES:
        raise ValueError(f"unsupported architecture: {arch}")
    value = digest(value)
    directory = Path(directory)
    directory.mkdir(parents=True, exist_ok=True)
    (directory / f"{arch}.digest").write_text(value + "\n", encoding="utf-8")


def references(directory, image):
    directory = Path(directory)
    expected = {f"{arch}.digest" for arch in ARCHES}
    if {p.name for p in directory.glob("*.digest")} != expected:
        raise ValueError("exactly amd64.digest and arm64.digest are required")
    values = [digest((directory / name).read_text(encoding="utf-8").strip())
              for name in (f"{arch}.digest" for arch in ARCHES)]
    if len(set(values)) != len(ARCHES):
        raise ValueError("platform digests must be distinct")
    if not image or "@" in image or any(c.isspace() for c in image):
        raise ValueError("invalid image name")
    return [f"{image}@{value}" for value in values]


def verify_index(index):
    if not isinstance(index, dict) or index.get("schemaVersion") != 2 or index.get("mediaType") not in INDEX_TYPES:
        raise ValueError("a multi-platform image index is required")
    if not isinstance(index.get("manifests"), list):
        raise ValueError("index manifests must be a list")
    platforms = {}
    attestations = []
    for entry in index.get("manifests", []):
        if not isinstance(entry, dict) or not isinstance(entry.get("platform"), dict):
            raise ValueError("each manifest needs a platform descriptor")
        value = digest(entry.get("digest"))
        platform = entry.get("platform", {})
        os_name, arch = platform.get("os"), platform.get("architecture")
        if os_name == "linux" and arch in ARCHES:
            if arch in platforms:
                raise ValueError(f"duplicate platform: linux/{arch}")
            platforms[arch] = value
        elif os_name == "unknown" and arch == "unknown":
            annotations = entry.get("annotations", {})
            if not isinstance(annotations, dict) or annotations.get("vnd.docker.reference.type") != "attestation-manifest":
                raise ValueError("unknown descriptor is not an attestation")
            attestations.append(annotations.get("vnd.docker.reference.digest"))
        else:
            raise ValueError(f"unexpected platform: {os_name}/{arch}")
    if set(platforms) != set(ARCHES):
        raise ValueError("linux/amd64 and linux/arm64 must both be present")
    if any(value not in platforms.values() for value in attestations):
        raise ValueError("attestation does not reference a target platform")
    return sorted(f"linux/{arch}" for arch in platforms)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    upload = commands.add_parser("record")
    upload.add_argument("arch")
    upload.add_argument("digest")
    upload.add_argument("directory")
    refs = commands.add_parser("refs")
    refs.add_argument("directory")
    refs.add_argument("image")
    verify = commands.add_parser("verify")
    verify.add_argument("index")
    args = parser.parse_args()
    try:
        if args.command == "record":
            record(args.arch, args.digest, args.directory)
        elif args.command == "refs":
            print("\n".join(references(args.directory, args.image)))
        else:
            print("Verified platforms: " + ", ".join(verify_index(json.loads(Path(args.index).read_text()))))
    except (ValueError, OSError) as error:
        print(str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
