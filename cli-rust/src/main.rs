extern crate clap;
extern crate dirs;

use clap::{App, Arg, SubCommand};
use std::fs;
use std::fs::File;
use std::io::Write;

fn main() {
    let author =
        "BlankOn Developer <blankon-dev@googlegroups.com>\nHerpiko Dwi Aguno <herpiko@aguno.xyz>";
    let version = "0.0.1";

    let matches = App::new("irgsh-cli")
    								.version(version)
    								.author(author)
    								.about("IRGSH command line interface")
    								.subcommand(SubCommand::with_name("init")
    									.about("Initializes the command line interface program for the first time. You need the IRGSH chief URL address.")
    									.version(version)
    									.author(author)
    									.arg(Arg::with_name("chief")
    										.short("chief")
    										.long("chief")
    										.value_name("URL")
    										.help("Sets the IRGSH Chief address")
                        .required(true)
    										.takes_value(true)))
    								.subcommand(SubCommand::with_name("submit")
    									.about("Submits the package and source (optional).")
    									.version(version)
    									.author(author)
    									.arg(Arg::with_name("package")
    										.short("-p")
    										.long("package")
    										.value_name("URL")
    										.help("Package Git URL")
                        .required(true)
    										.takes_value(true))
    									.arg(Arg::with_name("source")
    										.short("-s")
    										.long("source")
    										.value_name("URL")
    										.help("Source Git URL")
    										.takes_value(true)))
    								.subcommand(SubCommand::with_name("status")
    									.about("Checks the status of a pipeline.")
    									.version(version)
    									.author(author)
    									.arg(Arg::with_name("PIPELINE_ID")
                        .help("Pipeline ID")
                        .required(true)
                        .index(1)))
    								.subcommand(SubCommand::with_name("status")
    									.about("Watch the latest log of the pipeline in real time.")
    									.version(version)
    									.author(author)
    									.arg(Arg::with_name("PIPELINE_ID")
                        .help("Pipeline ID")
                        .required(true)
                        .index(1)))
    								.get_matches();
    let home_dir_path = dirs::home_dir().unwrap();
    let mut config_file = home_dir_path.into_os_string().into_string().unwrap();

    if let Some(matches) = matches.subcommand_matches("init") {
        let mut path_str = "/.irgsh";
        config_file.push_str(&path_str);
        fs::create_dir(&config_file).ok();
        path_str = "/IRGSH_CHIEF_ADDRESS";
        config_file.push_str(&path_str);
        // TOOD validate URL
        let url = matches.value_of("chief").unwrap();
        let mut f = File::create(&config_file).expect("Unable to create file");
        f.write_all(url.as_bytes()).expect("Unable to write data");
        println!(
            "Successfully sets the chief address to {}. Now you can use irgsh-cli.",
            matches.value_of("chief").unwrap()
        );
        return;
    }

    config_file.push_str("/.irgsh/IRGSH_CHIEF_ADDRESS");
    let chief_url = fs::read_to_string(&config_file).expect("Unable to read config file. Please initialize the irgsh-cli first. See --help for further information.");

    if let Some(matches) = matches.subcommand_matches("submit") {
        println!("Chief       : {}", chief_url);
        println!("Package URL : {}", matches.value_of("package").unwrap());
        return;
    } else if let Some(matches) = matches.subcommand_matches("status") {
        println!(
            "Status of PipelineID: {}",
            matches.value_of("PIPELINE_ID").unwrap()
        );
    } else if let Some(matches) = matches.subcommand_matches("watch") {
        println!(
            "Watching PipelineID: {}",
            matches.value_of("PIPELINE_ID").unwrap()
        );
    // Fall back to status subcommand if the pipeline was done.
    } else {
        println!("\nPlease run by a subcommand. See --help for further information.");
    }
}
