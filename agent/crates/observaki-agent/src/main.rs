//! Placeholder binary for the Observaki host agent.
//!
//! The real agent (collection, sampling and shipping) lands in Phase 2. This
//! crate exists from Phase 0 so the workspace, toolchain and release plumbing
//! are exercised before the agent is implemented.

use std::env;
use std::process::ExitCode;

const USAGE: &str = "\
observaki-agent — Observaki host agent (Phase 2 placeholder)

Usage:
  observaki-agent            print name and version
  observaki-agent --version  print name and version
  observaki-agent --help     show this help
";

fn main() -> ExitCode {
    let args: Vec<String> = env::args().skip(1).collect();

    match args.first().map(String::as_str) {
        None | Some("--version" | "-V") => {
            println!("{}", version_line());
            ExitCode::SUCCESS
        }
        Some("--help" | "-h") => {
            print!("{USAGE}");
            ExitCode::SUCCESS
        }
        Some(other) => {
            eprintln!("observaki-agent: unknown argument {other:?}\n");
            eprint!("{USAGE}");
            ExitCode::from(2)
        }
    }
}

fn version_line() -> String {
    format!("observaki-agent {}", env!("CARGO_PKG_VERSION"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn version_line_includes_name_and_manifest_version() {
        let line = version_line();
        assert!(line.starts_with("observaki-agent "));
        assert!(line.ends_with(env!("CARGO_PKG_VERSION")));
    }

    #[test]
    fn usage_documents_the_version_flag() {
        assert!(USAGE.contains("--version"));
    }
}
