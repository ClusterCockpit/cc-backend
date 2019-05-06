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
use Getopt::Long;
use Pod::Usage;
use DateTime::Format::Strptime;
use DBI;

my $database = 'jobDB';
my @conditions;
my ($add, $from, $to);

my $dateParser =
DateTime::Format::Strptime->new(
    pattern => '%d.%m.%Y',
    time_zone => 'Europe/Berlin',
    on_error  => 'undef'
);

my $help = 0;
my $man = 0;
my $hasprofile = '';
my $mode = 'count';
my $user = '';
my $project = '';
my @numnodes;
my @starttime;
my @duration;
my @mem_used;
my @mem_bandwidth;
my @flops_any;

GetOptions (
    'help'           => \$help,
    'man'            => \$man,
    'hasprofile=s'   => \$hasprofile,
    'mode=s'         => \$mode,
    'user=s'         => \$user,
    'project=s'      => \$project,
    'numnodes=i{2}'  => \@numnodes,
    'starttime=s{2}' => \@starttime,
    'duration=s{2}'  => \@duration,
    'mem_used=i{2}'  => \@mem_used,
    'mem_bandwidth=i{2}'  => \@mem_bandwidth,
    'flops_any=i{2}'  => \@flops_any
) or pod2usage(2);

my %attr = (
    PrintError => 1,
    RaiseError => 1
);

if ( $#ARGV == 0 ) {
    $database = $ARGV[0];
}

my $dbh = DBI->connect(
    "DBI:SQLite:dbname=$database", "", "", \%attr)
 or die("Cannot connect to database $database\n");

sub parseDate {
    my $str = shift;
    my $dt;

    if ( $str ){
        $dt = $dateParser->parse_datetime($str);

        if ( $dt ) {
            return $dt->epoch;
        } else {
            print "Cannot parse datetime string $str: Ignoring!\n";
            return 0;
        }
    } else {
        return 0;
    }
}

sub parseDuration {
    my $str = shift;

    if ( $str =~ /([0-9]+)h/ ) {
        return $1 * 3600;

    } elsif ( $str =~ /([0-9]+)m/ ) {
        return $1 * 60;

    } elsif ( $str =~ /([0-9]+)s/ ) {
        return $1;

    } elsif ( $str =~ /([0-9]+)/ ) {
        return $1;

    } else {
        print "Cannot parse duration string $str: Ignoring!\n";
        return 0;
    }
}

sub formatDuration {
    my $ts = shift;

}

sub processRange {
    my $lower = shift;
    my $upper = shift;

    if ( $lower && $upper ){
        return (3, $lower, $upper);
    } elsif ( $lower && !$upper ){
        return (1, $lower, 0);
    } elsif ( !$lower && $upper ){
        return (2, 0, $upper);
    }
}

sub buildCondition {
    my $name = shift;

    if ( $add ) {
        if ( $add == 1 ) {
            push @conditions, "$name < $from";
        } elsif ( $add == 2 ) {
            push @conditions, "$name > $to";
        } elsif ( $add == 3 ) {
            push @conditions, "$name BETWEEN $from AND $to";
        }
    }
}

sub printJobStat {
    my $conditionstring = shift;

    my $query = 'SELECT COUNT(id), SUM(duration)/3600, SUM(duration*num_nodes)/3600 FROM job '.$conditionstring;
    my ($count, $walltime, $nodeHours) = $dbh->selectrow_array($query);

    if ( $count > 0 ) {
        print "=================================\n";
        print "Job count: $count\n";
        print "Total walltime [h]: $walltime \n";
        print "Total node hours [h]: $nodeHours \n";

        $query = 'SELECT num_nodes, COUNT(*) FROM job '.$conditionstring.' GROUP BY 1';
        my @histo_num_nodes = $dbh->selectall_array($query);
        print "\nHistogram: Number of nodes\n";
        print "nodes\tcount\n";

        foreach my $bin ( @histo_num_nodes ) {
            print "$bin->[0]\t$bin->[1]\n";
        }

        $query = 'SELECT duration/3600, COUNT(*) FROM job '.$conditionstring.' GROUP BY 1';
        my @histo_runtime = $dbh->selectall_array($query);
        print "\nHistogram: Walltime\n";
        print "hours\tcount\n";

        foreach my $bin ( @histo_runtime ) {
            print "$bin->[0]\t$bin->[1]\n";
        }
    } else {
        print "No jobs\n";
    }
}


sub printJob {
    my $job = shift;

    my $jobString = <<"END_JOB";
=================================
JobId: $job->{job_id}
UserId: $job->{user_id}
Number of nodes: $job->{num_nodes}
From $job->{start_time} to $job->{stop_time}
Duration $job->{duration}
END_JOB

    print $jobString;
}

pod2usage(1) if $help;
pod2usage(-verbose  => 2) if $man;

# build query conditions
if ( $user ) {
    push @conditions, "user_id=\'$user\'";
}

if ( $project ) {
    push @conditions, "project_id=\'$project\'";
}


if ( @numnodes ) {
    ($add, $from, $to) = processRange($numnodes[0], $numnodes[1]);
    buildCondition('num_nodes');
}

if ( @starttime ) {
    ($add, $from, $to) = processRange( parseDate($starttime[0]), parseDate($starttime[1]));
    buildCondition('start_time');
}

if ( @duration ) {
    ($add, $from, $to) = processRange( parseDuration($duration[0]), parseDuration($duration[1]));
    buildCondition('duration');
}

if ( @mem_used ) {
    $hasprofile = 'true';
    ($add, $from, $to) = processRange($mem_used[0], $mem_used[1]);
    buildCondition('mem_used');
}

if ( @mem_bandwidth ) {
    $hasprofile = 'true';
    ($add, $from, $to) = processRange($mem_bandwidth[0], $mem_bandwidth[1]);
    buildCondition('mem_bw');
}

if ( @flops_any ) {
    $hasprofile = 'true';
    ($add, $from, $to) = processRange($flops_any[0], $flops_any[1]);
    buildCondition('flops_any');
}

if ( $hasprofile ) {
    if ( $hasprofile eq 'true' ) {
        push @conditions, "has_profile=1";
    } elsif ( $hasprofile eq 'false' ) {
        push @conditions, "has_profile=0";
    } else {
        print "Unknown value for option has_profile: $hasprofile. Can be true or false.\n";
    }
}

my $query;
my $conditionstring;

if ( @conditions ){
    $conditionstring = ' WHERE ';
    $conditionstring .= join(' AND ',@conditions);
}

# handle mode
if ( $mode eq 'query' ) {
    $query = 'SELECT * FROM job'.$conditionstring;
    print "$query\n";
    exit;
}

if ( $mode eq 'count' ) {
    $query = 'SELECT COUNT(*) FROM job'.$conditionstring;
    my ($count) = $dbh->selectrow_array($query);
    print "COUNT $count\n";
    exit;
}

if ( $mode eq 'stat' ) {
    printJobStat($conditionstring);
    exit;
}

$query = 'SELECT * FROM job'.$conditionstring;
my $sth = $dbh->prepare($query);
$sth->execute;
my %row;
$sth->bind_columns( \( @row{ @{$sth->{NAME_lc} } } ));

if ( $mode eq 'list' ) {
    while ($sth->fetch) {
        printJob(\%row);
    }
} elsif ( $mode eq 'ids' ) {
    while ($sth->fetch) {
        print "$row{job_id}\n";
    }
} else {
    die "ERROR Unknown mode $mode!\n";
}

__END__

=head1 NAME

acQuery.pl - Wrapper script to access sqlite job database.

=head1 SYNOPSIS

   acQuery.pl [options] -- <DB file>

   Help Options:
   --help  Show help text
   --man   Show man page
   --mode <mode>  Set the operation mode
   --user <user_id> Search for jobs of specific user
   --project <project_id> Search for jobs of specific project
   --duration <from> <to>  Specify duration range of jobs
   --numnodes <from> <to>  Specify range for number of nodes of job
   --starttime <from> <to>  Specify range for start time of jobs

=head1 OPTIONS

=over 8

=item B<--help>
Show a brief help information.

=item B<--man>
Read the manual, with examples

=item B<--mode [012]>
Specify output mode. Mode can be one of:

=over 4

=item B<ids>
Print list of job ids matching conditions. One job id per line. (default mode)

=item B<query>
Print the query string and then exit.

=item B<count>
Only output the number of jobs matching the conditions.

=item B<list>
Output a record of every job matching the conditions.

=item B<stat>
Output job statistic for all jobs matching the conditions.

=back

=item B<--user>
Search job for a specific user id.

=item B<--project>
Search job for a specific project.

=item B<--duration>
Specify condition for job duration. This option takes two arguments: If both
arguments are positive integers the condition is duration between first
argument and second argument. If the second argument is zero condition is duration
smaller than first argument. If first argument is zero condition is duration
larger than second argument. Duration can be in seconds, minutes (append m) or
hours (append h).

=item B<--numnodes>
Specify condition for number of node range of job. This option takes two
arguments: If both arguments are positive integers the condition is number of
nodes between first argument and second argument. If the second argument is
zero condition is number of nodes smaller than first argument. If first
argument is zero condition is number of nodes larger than second argument.

=item B<--starttime>
Specify condition for the starttime of job. This option takes two
arguments: If both arguments are positive integers the condition is start time
between first argument and second argument. If the second argument is
zero condition is start time smaller than first argument. If first
argument is zero condition is start time larger than second argument.
Start time must be given as date in the following format: %d.%m.%Y

=back

=head1 DESCRIPTION

=head1 EXAMPLES

=head1 AUTHOR

Jan Eitzinger - L<https://hpc.fau.de/person/jan-eitzinger/>

=cut
