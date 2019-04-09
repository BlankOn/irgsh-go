extern crate clap;

use clap::{Arg, App};

fn main() {
    let matches = App::new("cli-rust")
        .version("0.0.1")
        .author("BlankOn Developer <blankon-dev@googlegroups.com")
        .about("irgsh-cli tool written in Rust")
        .arg(Arg::with_name("init")
                .required(true)
                .takes_value(true)
                .index(1)
                .help("initialize the irgsh-cli"))
        .get_matches();
    let url = matches.value_of("init").unwrap();
    println!("{}", url);
}
