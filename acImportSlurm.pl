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

use Data::Dumper;
use DateTime::Format::Strptime;
use DBI;

if ( $#ARGV < 1 ){
    die "Usage: $0 <DBFile> <importDIR>\n";
}

my $database = $ARGV[0];
my $basedir = $ARGV[1];

my %attr = (
    PrintError => 1,
    RaiseError => 1
);

my $dbh = DBI->connect(
    "DBI:SQLite:dbname=$database", "", "", \%attr);

my $dateParser =
DateTime::Format::Strptime->new(
    pattern => '%Y-%m-%dT%H:%M:%S',
    time_zone => 'Europe/Berlin',
    on_error  => 'undef'
);

sub parse_nodelist {
    my $nodestr = shift;
    my @nodes;

    if ( $nodestr =~ /([a-z]+)\[(.*)\]/) {
        my $prefix = $1;
        my $list = $2;
        my @listitems = split(',', $list);

        foreach my $item ( @listitems ){
            if ( $item =~ /([0-9]+)-([0-9]+)/ ){
                foreach my $nodeId ( $1 ... $2 ){
                    push @nodes, $prefix.$nodeId;
                }
            } else {
                push @nodes, $prefix.$item;
            }
        }

        return join(',', @nodes);
    } else {
        return $nodestr;
    }
}

my $sth_insert_job = $dbh->prepare(qq{
    INSERT INTO job
    (job_id, user_id, project_id, cluster_id,
    start_time, stop_time, duration, walltime,
    job_state, num_nodes, node_list, has_profile)
    VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
    });

my $sth_select_job = $dbh->prepare(qq{
    SELECT id, user_id, job_id, cluster_id,
           start_time, stop_time, duration, num_nodes
    FROM job
    WHERE job_id=?
    });

my %JOBCACHE;
my $dt;

while( defined( my $file = glob($basedir . '/*' ) ) ) {

    print "Processing $file ...";
    open(my $fh, "<","$file");
    my $columns = <$fh>;

    while ( my $record = <$fh> ) {

        my @fields = split(/\|/, $record);

        if ( $fields[1] =~ /^[0-9]+$/) {

            my $cluster_id = $fields[0];
            my $job_id = $fields[1];
            my $user_id = $fields[2];
            my $project_id = $fields[3];
            $dt = $dateParser->parse_datetime($fields[5]);
            my $start_time = $dt->epoch;
            $dt = $dateParser->parse_datetime($fields[6]);
            my $stop_time = $dt->epoch;
            my $num_nodes = $fields[11];
            my $node_list = parse_nodelist($fields[13]);
            my $job_state = $fields[10];
            $job_state =~ s/ by [0-9]+//;
            my $walltime = 0;

            my $duration = $stop_time - $start_time;

            # check if job already exists
            my @row = $dbh->selectrow_array($sth_select_job, undef, $job_id);

            if ( @row ) {
                print "Job $job_id already exists!\n";
            } else {
                $sth_insert_job->execute(
                    $job_id,
                    $user_id,
                    $project_id,
                    $cluster_id,
                    $start_time,
                    $stop_time,
                    $duration,
                    $walltime,
                    $job_state,
                    $num_nodes,
                    $node_list,
                    0);
            }
        } else {
            # print "$fields[1] \n";
            next;
        }
    }

    close $fh or die "can't close file $!";
    print " done\n";
}

$dbh->disconnect;
