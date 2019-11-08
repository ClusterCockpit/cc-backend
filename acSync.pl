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

use File::Slurp;
use Data::Dumper;
use JSON::MaybeXS qw(encode_json decode_json);
use DBI;

my $database = $ARGV[0];
my $basedir = $ARGV[1];

my %attr = (
    PrintError => 1,
    RaiseError => 1
);

my $dbh = DBI->connect(
    "DBI:SQLite:dbname=$database", "", "", \%attr)
    or die "Could not connect to database: $DBI::errstr";

my $sth_select_job = $dbh->prepare(qq{
    SELECT id, user_id, job_id, cluster_id,
    start_time, stop_time, duration, num_nodes
    FROM job
    WHERE job_id=?
    });

my $jobcount = 0;
my $wrongjobcount = 0;

opendir my $dh, $basedir or die "can't open directory: $!";
while ( readdir $dh ) {
    chomp;
    next if $_ eq '.' or $_ eq '..';

    my $jobID = $_;
    my $needsUpdate = 0;

    my $jobmeta_json = read_file("$basedir/$jobID/meta.json");
    my $job = decode_json $jobmeta_json;
    my @row = $dbh->selectrow_array($sth_select_job, undef, $jobID);

    if ( @row ) {

        $jobcount++;
    # print Dumper(@row);
        my $duration_diff = abs($job->{duration} - $row[6]);

        if ( $duration_diff > 120 ) {
            $needsUpdate = 1;
            # print "$jobID DIFF DURATION $duration_diff\n";
            # print "CC $row[4] - $row[5]\n";
            # print "DB $job->{start_time} - $job->{stop_time}\n"
        }

        if ( $row[7] != $job->{num_nodes} ){
            $needsUpdate = 1;
            # print "$jobID DIFF NODES $row[7] $job->{num_nodes}\n";
        }
    } else {
        print "$jobID NOT in DB!\n";
    }

    if ( $needsUpdate ){
        $wrongjobcount++;
        print "$jobID\n";
    }
}
closedir $dh or die "can't close directory: $!";
$dbh->disconnect;

print "$wrongjobcount of $jobcount need update\n";
