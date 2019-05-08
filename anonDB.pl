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

my $sth_select_all = $dbh->prepare(qq{
    SELECT id, user_id, project_id
    FROM job;
    });

my $sth_select_job = $dbh->prepare(qq{
    SELECT id, user_id, project_id
    FROM job
    WHERE job_id=?
    });

my $sth_update_job = $dbh->prepare(qq{
    UPDATE job
    SET user_id = ?,
        project_id = ?,
        flops_any = ?,
        mem_bw = ?
    WHERE id=?;
    });

my ($user_id, $num_nodes, $start_time, $stop_time, $queue, $duration, $db_id);

# build user lookup
$sth_select_all->execute;
my $user_index = 0; my $project_index = 0;
my %user_lookup; my %project_lookup;
my %row;
$sth_select_all->bind_columns( \( @row{ @{$sth->{NAME_lc} } } ));

while ($sth_select_all->fetch) {
    print "$row{job_id}\n";
}



opendir my $dh, $basedir or die "can't open directory: $!";
while ( readdir $dh ) {
    chomp;
    next if $_ eq '.' or $_ eq '..';

    my $jobID = $_;
    my $needsUpdate = 0;

    my $jobmeta_json = read_file("$basedir/$jobID/meta.json");
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

        my $footprint = $job->{footprint};

        # print "$footprint->{mem_used}->{avg}, $footprint->{flops_any}->{avg}, $footprint->{mem_bw}->{avg}\n";

        $sth_update_job->execute(
            1,
            $footprint->{mem_used}->{avg},
            $footprint->{flops_any}->{avg},
            $footprint->{mem_bw}->{avg},
            $db_id
        );

        $jobcount++;
    } else {
        print "$jobID NOT in DB!\n";
    }
}
closedir $dh or die "can't close directory: $!";
$dbh->disconnect;

print "$wrongjobcount of $jobcount need update\n";
