#!/usr/bin/env perl

use strict;
use warnings;
use utf8;

use File::Copy;

my $trunk = '/home/jan/prg/HPCJobDatabase';
my $basedir = $ARGV[0];
my $destdir = $ARGV[1];


opendir my $dh, $basedir or die "can't open directory: $!";
while ( readdir $dh ) {
    use integer;
    chomp;
    next if $_ eq '.' or $_ eq '..';

    my $jobID = $_;
    my $srcPath = "$trunk/$basedir/$jobID";
    $jobID =~ s/\.eadm//;

    my $level1 = $jobID/1000;
    my $level2 = $jobID%1000;

    my $dstPath = sprintf("%s/%s/%d/%03d", $trunk, $destdir, $level1, $level2);
    # print "COPY from $srcPath to $dstPath\n";
    # print "$trunk/$destdir/$level1\n";
    if (not -d "$trunk/$destdir/$level1") {
        mkdir "$trunk/$destdir/$level1";
    }

    move($srcPath, $dstPath);
}

