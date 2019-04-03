#!/bin/sh
# This is an example script that can be hooked into reprepro
# to generate a hierachy like packages.debian.org/changelogs/
# All you have to do is to copy it into you conf/ directory,
# and add the following to any distribution in conf/distributions
# you want to have changelogs and copyright files extracted:
#Log:
# --type=dsc changelogs.example
# (note the space at the beginning of the second line).
# This will cause this script to extract changelogs for all
# newly added source packages. (To generate them for already
# existing packages, call "reprepro rerunnotifiers").

# DEPENDENCIES: dpkg >= 1.13.9

if test "x${REPREPRO_OUT_DIR:+set}" = xset ; then
	# Note: due to cd, REPREPRO_*_DIR will no longer
	# be usable. And only things relative to outdir will work...
	# (as the filekeys given to this are, though for changesfiles
	# and CAUSING_FILE it would be different)
	cd "${REPREPRO_OUT_DIR}" || exit 1
else
	cat "This script is written for reprepro 3.5.1 upwoards" >&2
	cat "for an older version, disable that warning and add" >&2
	cat "a proper cd or always use from the basedir" >&2
	exit 1
fi

# You will either always have to call it with the basedir as
# your current directory, use reprepro >= 3.5.1
# or uncomment and edit the following to be your base directory.

# cd /path/to/your/repository

# place where the changelogs should be put into:

CHANGELOGDIR=changelogs

# Set to avoid using some predefined TMPDIR or even /tmp as
# tempdir:

# TMPDIR=/var/cache/whateveryoucreated

addsource() {
	DSCFILE="$1"
	CANONDSCFILE="$(readlink --canonicalize "$DSCFILE")"
	TARGETDIR="$CHANGELOGDIR"/"$(echo $DSCFILE | sed -e 's/\.dsc$//')"
	SUBDIR="$(basename $TARGETDIR)"
	BASEDIR="$(dirname $TARGETDIR)"
	if ! [ -d "$TARGETDIR" ] ; then
		echo "extract $CANONDSCFILE information to $TARGETDIR"
		mkdir -p -- "$TARGETDIR"
		EXTRACTDIR="$(mktemp -d)"
		(cd -- "$EXTRACTDIR" && dpkg-source -sn -x "$CANONDSCFILE" > /dev/null)
		install -D -- "$EXTRACTDIR"/*/debian/copyright "$TARGETDIR/copyright"
		install -D -- "$EXTRACTDIR"/*/debian/changelog "$TARGETDIR/changelog"
		chmod -R u+rwX -- "$EXTRACTDIR"
		rm -r -- "$EXTRACTDIR"
	fi
	if [ -L "$BASEDIR"/current."$CODENAME" ] ; then
		# should not be there, just to be sure
		rm -f -- "$BASEDIR"/current."$CODENAME"
	fi
	# mark this as needed by this distribution
	ln -s -- "$SUBDIR" "$BASEDIR/current.$CODENAME"
	JUSTADDED="$TARGETDIR"
}
delsource() {
	DSCFILE="$1"
	TARGETDIR=changelogs/"$(echo $DSCFILE | sed -e 's/\.dsc$//')"
	SUBDIR="$(basename $TARGETDIR)"
	BASEDIR="$(dirname $TARGETDIR)"
	if [ "x$JUSTADDED" = "x$TARGETDIR" ] ; then
		exit 0
	fi
#	echo "delete, basedir=$BASEDIR targetdir=$TARGETDIR, dscfile=$DSCFILE, "
 	if [ "x$(readlink "$BASEDIR/current.$CODENAME")" = "x$SUBDIR" ] ; then
 		rm -- "$BASEDIR/current.$CODENAME"
 	fi
 	NEEDED=0
 	for c in "$BASEDIR"/current.* ; do
 		if [ "x$(readlink -- "$c")" = "x$SUBDIR" ] ; then
 			NEEDED=1
 		fi
 	done
 	if [ "$NEEDED" -eq 0 -a -d "$TARGETDIR" ] ; then
 		rm -r -- "$TARGETDIR"
		# to remove the directory if now empty
		rmdir --ignore-fail-on-non-empty -- "$BASEDIR"
 	fi
}

ACTION="$1"
CODENAME="$2"
PACKAGETYPE="$3"
if [ "x$PACKAGETYPE" != "xdsc" ] ; then
# the --type=dsc should cause this to never happen, but better safe than sorry.
	exit 1
fi
COMPONENT="$4"
ARCHITECTURE="$5"
if [ "x$ARCHITECTURE" != "xsource" ] ; then
	exit 1
fi
NAME="$6"
shift 6
JUSTADDED=""
if [ "x$ACTION" = "xadd" -o "x$ACTION" = "xinfo" ] ; then
	VERSION="$1"
	shift
	if [ "x$1" != "x--" ] ; then
		exit 2
	fi
	shift
	while [ "$#" -gt 0 ] ; do
		case "$1" in
			*.dsc)
				addsource "$1"
				;;
			--)
				exit 2
				;;
		esac
		shift
	done
elif [ "x$ACTION" = "xremove" ] ; then
	OLDVERSION="$1"
	shift
	if [ "x$1" != "x--" ] ; then
		exit 2
	fi
	shift
	while [ "$#" -gt 0 ] ; do
		case "$1" in
			*.dsc)
				delsource "$1"
				;;
			--)
				exit 2
				;;
		esac
		shift
	done
elif [ "x$ACTION" = "xreplace" ] ; then
	VERSION="$1"
	shift
	OLDVERSION="$1"
	shift
	if [ "x$1" != "x--" ] ; then
		exit 2
	fi
	shift
	while [ "$#" -gt 0 -a "x$1" != "x--" ] ; do
		case "$1" in
			*.dsc)
				addsource "$1"
			;;
		esac
		shift
	done
	if [ "x$1" != "x--" ] ; then
		exit 2
	fi
	shift
	while [ "$#" -gt 0 ] ; do
		case "$1" in
			*.dsc)
				delsource "$1"
				;;
			--)
				exit 2
				;;
		esac
		shift
	done
fi

exit 0
# Copyright 2007,2008 Bernhard R. Link <brlink@debian.org>
#
# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 2 as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA  02111-1301  USA
