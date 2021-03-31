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
    pattern => '%m/%d/%Y %H:%M:%S',
    time_zone => 'Europe/Berlin',
    on_error  => 'undef'
);

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

while( defined( my $file = glob($basedir . '/*' ) ) ) {

    print "Processing $file ...";
    open(my $fh, "<","$file");

    while ( my $record = <$fh> ) {
        if ( $record =~ /(.*);([A-Z]);(.*?);(.*)/ ) {
            my $dt = $dateParser->parse_datetime($1);
            my $timestamp = $dt->epoch;
            my $job_state = $2;
            my $job_id = $3;
            my $jobinfo = $4;
            my @data = split(/ /, $jobinfo);
            my $queue;
            my $user_id;
            my $project_id;
            my $start_time;
            my $stop_time;
            my $walltime;
            my @nodes;
            my $num_nodes;
            my $node_list;

            foreach my $prop ( @data ) {
                if ( $prop =~ /user=(.*)/ ) {
                    $user_id = $1;
                } elsif ( $prop =~ /group=(.*)/ ) {
                    $project_id = $1;
                } elsif ( $prop =~ /start=(.*)/ ) {
                    $start_time = $1;
                } elsif ( $prop =~ /end=(.*)/ ) {
                    $stop_time = $1;
                } elsif ( $prop =~ /queue=(.*)/ ) {
                    $queue = $1;
                } elsif ( $prop =~ /Resource_List\.walltime=([0-9]+):([0-9]+):([0-9]+)/ ) {
                    $walltime = $1 * 3600 + $2 * 60 + $3;
                } elsif ( $prop =~ /exec_host=(.*)/ ) {
                    my $hostlist = $1;
                    my @hosts = split(/\+/, $hostlist);

                    foreach my $host ( @hosts ) {
                        if ( $host =~ /(.*?)\/0/) {
                            push @nodes, $1;
                        }
                    }

                    $num_nodes = @nodes;
                    $node_list = join(',', @nodes);
                }
            }

            if ( $job_state eq 'S' ) {
                $JOBCACHE{$job_id}  = {
                    'user_id'      => $user_id,
                    'project_id'   => $project_id,
                    'start_time'   => $start_time,
                    'walltime'     => $walltime,
                    'num_nodes'    => $num_nodes,
                    'node_list'    => $node_list
                };
            } elsif ( $job_state eq 'E' ) {
                delete $JOBCACHE{$job_id};
            } elsif ( $job_state eq 'D' or $job_state eq 'A' ) {
                my $job;

                if (exists $JOBCACHE{$job_id}){
                    $job = $JOBCACHE{$job_id};
                } else {
                    next;
                }
                # print Dumper($job);
                $user_id     = $job->{'user_id'};
                $project_id  = $job->{'project_id'};
                $start_time  = $job->{'start_time'};
                $stop_time   = $timestamp;
                $walltime    = $job->{'walltime'};
                $num_nodes   = $job->{'num_nodes'};
                $node_list   = $job->{'node_list'};
                delete $JOBCACHE{$job_id};
            }

            if ( $job_state eq 'E' or
                 $job_state eq 'D' or
                 $job_state eq 'A' )
             {
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
                        "emmy",
                        $start_time,
                        $stop_time,
                        $duration,
                        $walltime,
                        $job_state,
                        $num_nodes,
                        $node_list,
                        0);
                }
            }
        }
    }

    close $fh or die "can't close file $!";
    print " done\n";
}

$dbh->disconnect;
