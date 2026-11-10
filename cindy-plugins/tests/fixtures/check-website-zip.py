"""Unpack Cindy's actual task file and resolve the static reader and feed links."""
import json
import sys
import tempfile
import zipfile
import xml.etree.ElementTree as ET
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urlparse, parse_qs


class Links(HTMLParser):
    def __init__(self):
        super().__init__()
        self.urls = []

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        self.urls.extend(attrs[key] for key in ("href", "src") if key in attrs)


with tempfile.TemporaryDirectory() as tmp, zipfile.ZipFile(sys.argv[1]) as archive:
    root = Path(tmp).resolve()
    names = archive.namelist()
    assert archive.testzip() is None and len(names) == len(set(names))
    for name in names:
        assert (root / name).resolve().is_relative_to(root) and "\\" not in name
    archive.extractall(root)
    site = json.loads((root / "data/site.json").read_bytes())
    pages = [p for chapter in site["chapters"] for p in chapter["pages"]]
    assert pages and site["missing_count"] == int(sys.argv[2])
    assert json.loads((root / "data/build.json").read_bytes())["generation_uuid"] == sys.argv[3]
    expected = {"index.html", "rss.xml", "README.md", "data/site.json", "data/site.js",
                "data/build.json", "static/style.css", "static/reader.js"} | {p["src"] for p in pages}
    assert set(names) == expected, set(names) ^ expected
    links = Links()
    links.feed((root / "index.html").read_text())
    assert set(url for url in links.urls if url.startswith("./")) == {
        "./rss.xml", "./static/style.css", "./static/reader.js", "./data/site.js"}
    for page in pages:
        assert (root / page["src"]).read_bytes().startswith(b"\xff\xd8")
    assert (root / "data/site.js").read_text().startswith("window.LUMI_WEBSITE = ")
    # The generated RSS uses the desktop template. Resolve it as on a real
    # deployment under a subpath; never write a public hostname into the ZIP.
    rss = (root / "rss.xml").read_text()
    base = "https://example.test/book"
    assert "{site_url}" in rss
    channel = ET.fromstring(rss.replace("{site_url}", base)).find("channel")
    assert channel.find("{http://www.w3.org/2005/Atom}link").get("href") == base + "/rss.xml"
    chapters = [chapter for chapter in site["chapters"] if chapter["pages"]]
    items = channel.findall("item")
    assert len(chapters) == len(items)
    for chapter, item in zip(chapters, items):
        url = urlparse(item.findtext("link"))
        assert url.path == "/book/index.html" and parse_qs(url.fragment)["chapter"] == [chapter["uuid"]]
        links = Links()
        links.feed(item.findtext("description"))
        assert base + "/" + chapter["pages"][0]["src"] in links.urls
        assert all((root / urlparse(link).path.removeprefix("/book/")).is_file() for link in links.urls)
