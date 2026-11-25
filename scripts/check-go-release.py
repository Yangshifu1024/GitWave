import json
import os
import re
import sys


def parse_version(value):
    if len(value) > 80 or not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", value):
        raise SystemExit(f"{value!r} must be a numeric X.Y.Z version")
    return tuple(map(int, value.split(".")))


release = parse_version(os.environ["RELEASE_VERSION"])
exporter = parse_version(os.environ["EXPORTER_VERSION"])
bridge = parse_version(os.environ["BRIDGE_VERSION"])
if not (0, 11, 0) < exporter < bridge <= release:
    raise SystemExit("Require v0.11.0 < preference exporter < Go bridge <= release")

versions = []
go_release_feeds = {"gitwave-update.json", "gitwave-legacy-transition.json"}
for page in json.load(sys.stdin):
    for item in page:
        if item["draft"] or item["prerelease"]:
            continue
        if not any(asset["name"] in go_release_feeds for asset in item["assets"]):
            continue
        tag = item["tag_name"]
        if not tag.startswith("v"):
            raise SystemExit(f"The published Go release has an unexpected tag: {tag!r}")
        versions.append((parse_version(tag[1:]), tag))

if not versions:
    if bridge != release:
        raise SystemExit("The first Go release must also be the Go bridge")
else:
    previous, previous_tag = max(versions)
    if release <= previous or bridge == release:
        raise SystemExit(f"The Go release must advance beyond {previous_tag} and keep the existing bridge")
    print(previous_tag)
