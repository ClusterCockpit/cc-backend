#!/usr/bin/env perl
# =======================================================================================
#
#      Author:   Jan Eitzinger (je), jan.eitzinger@fau.de
#      Copyright (c) 2019 RRZE, University Erlangen-Nuremberg
#
#      Permission is hereby granted, free of charge, to any person obtaining a copy
#      of this software and associated documentation files (the "Software"), to deal
#      in the Software without restriction, including without limitation the rights
#      to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#      copies of the Software, and to permit persons to whom the Software is
#      furnished to do so, subject to the following conditions:
#
#      The above copyright notice and this permission notice shall be included in all
#      copies or substantial portions of the Software.
#
#      THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
#      IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
#      FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
#      AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
#      LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
#      OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
#      SOFTWARE.
#
# =======================================================================================

use strict;
use warnings;
use utf8;

my $basedir = $ARGV[0];
open(my $fh, '>:encoding(UTF-8)', 'jobIds.txt')
    or die "Could not open file  $!";

opendir my $odh, $basedir or die "can't open directory: $!";

while ( readdir $odh ) {
    chomp;
    next if $_ eq '.' or $_ eq '..';

    my $jobID1 = $_;
    print "Open $jobID1\n";

    opendir my $idh, "$basedir/$jobID1" or die "can't open directory: $!";

    while ( readdir $idh ) {
        chomp;
        next if $_ eq '.' or $_ eq '..';
        my $jobID2 = $_;

        print $fh "$jobID1$jobID2.eadm $jobID1 $jobID2\n";
    }

    closedir $idh or die "can't close directory: $!";
}
closedir $odh or die "can't close directory: $!";
close $fh;
