#!/usr/bin/env python

#############################################################################
# generates partial package updates list as reprepro hook
# (to be used by apt-get >= 0.6.44, apt-qupdate or things compatible with that)

# changes Copyright 2005 Bernhard R. Link <brlink@debian.org>
# as this is used as hook, it does not need any parsing of
# Configuration or Handling of architectures and components.
# Also reprepro will present old and new file, so it does not
# need to store a permanent copy of the last version.
# This needs either python-apt installed or you have to change
# it to use another sha1 calculation method.

# HOW TO USE:
# - install python-apt
# - make sure your paths contain no ' characters.
# - be aware this is still quite experimental and might not
#   report some errors properly
# - copy this file to your conf/ directory
# - uncompress this file if it is compressed
# - make it executeable
# - add something like the following to the every distribution
#   in conf/distributions you want to have diffs for:
#
# DscIndices: Sources Release . .gz tiffany
# DebIndices: Packages Release . .gz tiffany
# 
# The first line is for source indices, the second for binary indices.
# Make sure uncompressed index files are generated (the single dot in those
# lines), as this version only diffs the uncompressed files.

# This file is a heavily modified version of apt-qupdate's tiffany,
# (downloaded from http://ftp-master.debian.org/~ajt/tiffani/tiffany
#  2005-02-20)which says:
#--------------------------------------------------------------------
# idea and basic implementation by Anthony, some changes by Andreas
# parts are stolen from ziyi
#
# Copyright (C) 2004-5  Anthony Towns <aj@azure.humbug.org.au>
# Copyright (C) 2004-5  Andreas Barth <aba@not.so.argh.org>
#--------------------------------------------------------------------

# This program is free software; you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation; either version 2 of the License, or
# (at your option) any later version.

# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.

# You should have received a copy of the GNU General Public License
# along with this program; if not, write to the Free Software
# Foundation, Inc., 59 Temple Place, Suite 330, Boston, MA  02111-1307  USA

#############################################################################

import sys, os, time
import apt_pkg

################################################################################

def usage (exit_code=0):
    print """Usage: tiffani directory newfile oldfile mode 3>releaselog
Write out ed-style diffs to Packages/Source lists
This file is intended to be called by reprepro as hook
given to DebIndices, UDebIndices or DscIndices.

    """
    sys.exit(exit_code);


def tryunlink(file):
    try:
        os.unlink(file)
    except OSError:
        print "warning: removing of %s denied" % (file)

class Updates:
    def __init__(self, readpath = None):
        self.can_path = None
        self.history = {}
        self.max = 14
        self.readpath = readpath
        self.filesizesha1 = None

        if readpath:
          try:
            f = open(readpath + "/Index")
            x = f.readline()

            def read_hashs(ind, f, self, x=x):
                while 1:
                    x = f.readline()
                    if not x or x[0] != " ": break
                    l = x.split()
                    if not self.history.has_key(l[2]):
                        self.history[l[2]] = [None,None]
                    self.history[l[2]][ind] = (l[0], int(l[1]))
                return x

            while x:
                l = x.split()

                if len(l) == 0:
                    x = f.readline()
                    continue

                if l[0] == "SHA1-History:":
                    x = read_hashs(0,f,self)
                    continue

                if l[0] == "SHA1-Patches:":
                    x = read_hashs(1,f,self)
                    continue

                if l[0] == "Canonical-Name:" or l[0]=="Canonical-Path:":
                    self.can_path = l[1]

                if l[0] == "SHA1-Current:" and len(l) == 3:
                    self.filesizesha1 = (l[1], int(l[2]))

                x = f.readline()

          except IOError:
            0

    def dump(self, out=sys.stdout):
        if self.can_path:
            out.write("Canonical-Path: %s\n" % (self.can_path))
        
        if self.filesizesha1:
            out.write("SHA1-Current: %s %7d\n" % (self.filesizesha1))

        hs = self.history
        l = self.history.keys()
        l.sort()

        cnt = len(l)
        if cnt > self.max:
            for h in l[:cnt-self.max]:
                tryunlink("%s/%s.gz" % (self.readpath, h))
                del hs[h]
            l = l[cnt-self.max:]

        out.write("SHA1-History:\n")
        for h in l:
            out.write(" %s %7d %s\n" % (hs[h][0][0], hs[h][0][1], h))
        out.write("SHA1-Patches:\n")
        for h in l:
            out.write(" %s %7d %s\n" % (hs[h][1][0], hs[h][1][1], h))

def sizesha1(f):
    size = os.fstat(f.fileno())[6]
    f.seek(0)
    sha1sum = apt_pkg.sha1sum(f)
    return (sha1sum, size)

def getsizesha1(name):
	f = open(name, "r")
	r = sizesha1(f)
	f.close()
	return r

def main():
    if len(sys.argv) != 5:
    	usage(1)

    directory = sys.argv[1]
    newrelfile = sys.argv[2]
    oldrelfile = sys.argv[3]
    mode = sys.argv[4]

    # this is only needed with reprepro <= 0.7
    if oldrelfile.endswith(".gz"):
    	sys.exit(0);


    oldfile = "%s/%s" % (directory,oldrelfile)
    newfile= "%s/%s" % (directory,newrelfile)

    outdir = oldfile + ".diff"

    if mode == "old":
    	# Nothing to do...
    	if os.path.isfile(outdir + "/Index"):
		os.write(3,oldrelfile + ".diff/Index")
    	sys.exit(0);

    if mode == "new":
    	# TODO: delete possible existing Index and patch files?
    	sys.exit(0);

    print "making diffs between %s and %s: " % (oldfile, newfile)

    o = os.popen("date +%Y-%m-%d-%H%M.%S")
    patchname = o.readline()[:-1]
    o.close()
    difffile = "%s/%s" % (outdir, patchname)

    upd = Updates(outdir)

    oldsizesha1 = getsizesha1(oldfile)

    # should probably early exit if either of these checks fail
    # alternatively (optionally?) could just trim the patch history

    if upd.filesizesha1:
        if upd.filesizesha1 != oldsizesha1:
            print "old file seems to have changed! %s %s => %s %s" % (upd.filesizesha1 + oldsizesha1)
    	    sys.exit(1);

    newsizesha1 = getsizesha1(newfile)

    if newsizesha1 == oldsizesha1:
        print "file unchanged, not generating diff"
    	if os.path.isfile(outdir + "/Index"):
		os.write(3,oldrelfile + ".diff/Index\n")
    else:
        if not os.path.isdir(outdir): os.mkdir(outdir)
        print "generating diff"
	while os.path.isfile(difffile + ".gz"):
		print "This was too fast, diffile already there, waiting a bit..."
		time.sleep(2)
		o = os.popen("date +%Y-%m-%d-%H%M.%S")
		patchname = o.readline()[:-1]
		o.close()
		difffile = "%s/%s" % (outdir, patchname)

	# TODO make this without shell...
	os.system("diff --ed '%s' '%s' > '%s'" % 
                         (oldfile,newfile, difffile))
        difsizesha1 = getsizesha1(difffile)
	# TODO dito
	os.system("gzip -9 '%s'" %difffile)


        upd.history[patchname] = (oldsizesha1, difsizesha1)
        upd.filesizesha1 = newsizesha1

        f = open(outdir + "/Index.new", "w")
        upd.dump(f)
        f.close()
# Specifing the index should be enough, it contains checksums for the diffs
	os.write(3,oldrelfile + ".diff/Index.new\n")

################################################################################

if __name__ == '__main__':
    main()
