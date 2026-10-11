import { readFileSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { resolve } from "node:path";

// Tauri requires paired npm packages and Rust crates to share major/minor.
// Compare resolved versions, not the permissive ranges in package.json.
export function findVersionProblems(npmVersions, cargoLock) {
  const crates = new Map();
  for (const block of cargoLock.split(/^\[\[package\]\]$/m).slice(1)) {
    const name = block.match(/^name = "([^"]+)"$/m)?.[1];
    const version = block.match(/^version = "([^"]+)"$/m)?.[1];
    if (name && version) {
      const versions = crates.get(name) ?? [];
      versions.push(version);
      crates.set(name, versions);
    }
  }

  const problems = [];
  for (const [name, version] of Object.entries(npmVersions)) {
    const crate = name === "@tauri-apps/api" ? "tauri" : name.replace("@tauri-apps/", "tauri-");
    const rustVersions = crates.get(crate);
    if (!rustVersions?.length) {
      problems.push(`${name}: missing ${crate} in Cargo.lock`);
      continue;
    }
    const npmMinor = version.match(/^\d+\.\d+(?=\.)/)?.[0];
    if (!npmMinor || rustVersions.some((rust) => rust.match(/^\d+\.\d+(?=\.)/)?.[0] !== npmMinor)) {
      problems.push(`${crate} (${rustVersions.join(", ")}) : ${name} (${version})`);
    }
  }
  return problems;
}

function main() {
  const root = fileURLToPath(new URL("../", import.meta.url));
  const pkg = JSON.parse(readFileSync(resolve(root, "package.json"), "utf8"));
  const npmVersions = {};
  for (const name of Object.keys(pkg.dependencies)) {
    if (name === "@tauri-apps/api" || name.startsWith("@tauri-apps/plugin-")) {
      npmVersions[name] = JSON.parse(
        readFileSync(resolve(root, "node_modules", name, "package.json"), "utf8"),
      ).version;
    }
  }
  const problems = findVersionProblems(
    npmVersions,
    readFileSync(resolve(root, "src-tauri/Cargo.lock"), "utf8"),
  );
  if (problems.length) {
    console.error(`Tauri npm/Rust major/minor versions must match:\n${problems.join("\n")}`);
    process.exitCode = 1;
  } else {
    console.log("Tauri npm/Rust versions match.");
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  main();
}
