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
    start_time, stop_time, duration, num_nodes,
    has_profile, mem_used_max, flops_any_avg, mem_bw_avg,
    ib_bw_avg, file_bw_avg
    FROM job
    WHERE job_id=?
    });

my $sth_update_job = $dbh->prepare(qq{
    UPDATE job
    SET has_profile = ?,
        mem_used_max = ?,
        flops_any_avg = ?,
        mem_bw_avg = ?
    WHERE id=?;
    });

my $jobcount = 0;
my $wrongjobcount = 0;
my ($TS, $TE);
my $counter = 0;

open(my $fh, '<:encoding(UTF-8)', './jobIds.txt')
    or die "Could not open file  $!";
$TS = time();

while ( <$fh> ) {

    my ($jobID, $path1, $path2) = split ' ', $_;
    $counter++;

    my $jobmeta_json = read_file("$basedir/$path1/$path2/meta.json");
    my $job = decode_json $jobmeta_json;
    my @row = $dbh->selectrow_array($sth_select_job, undef, $jobID);
    my ($db_id, $db_user_id, $db_job_id, $db_cluster_id, $db_start_time, $db_stop_time, $db_duration, $db_num_nodes);

    # print Dumper($job);

    if ( @row ) {
        ($db_id,
            $db_user_id,
            $db_job_id,
            $db_cluster_id,
            $db_start_time,
            $db_stop_time,
            $db_duration,
            $db_num_nodes) = @row;

        my $stats = $job->{statistics};

        if ( $job->{user_id} ne $db_user_id ) {
            print "jobID $jobID $job->{user_id} $db_user_id\n";
            $job->{user_id} = $db_user_id;
        }

        # if ( $job->{start_time} != $db_start_time ) {
        #     print "start $jobID $job->{start_time} $db_start_time\n";
        # }
        # if ( $job->{stop_time} != $db_stop_time ) {
        #     print "stop $jobID $job->{stop_time} $db_stop_time\n";
        # }
        if ( $job->{duration} != $db_duration ) {
            my $difference = $job->{duration} - $db_duration;
            if ( abs($difference) > 120 ) {
                print "duration $jobID $job->{duration} $db_duration $difference\n";
            }
        }

        $sth_update_job->execute(
            1,
            $stats->{mem_used}->{max},
            $stats->{flops_any}->{avg},
            $stats->{mem_bw}->{avg},
            $db_id
        );

        $jobcount++;
    } else {
        print "$jobID NOT in DB!\n";
    }

    if ( $counter == 50 ) {
        $TE = time();
        my $rate = $counter/($TE-$TS);
        $counter = 0;
        print "Processing $rate jobs per second\n";
        $TS = $TE;
    }
}
$dbh->disconnect;
close $fh;

print "$wrongjobcount of $jobcount need update\n";
