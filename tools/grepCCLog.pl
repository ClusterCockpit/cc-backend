#!/usr/bin/env perl

my $filename = $ARGV[0];
my $Tday = $ARGV[1];

open FILE,"<$filename";

my %startedJob;
my %stoppedJob;

foreach ( <FILE> ) {
    if ( /Oct ([0-9]+) .*new job \(id: ([0-9]+)\): cluster=([a-z]+), jobId=([0-9]+), user=([a-z0-9]+),/ ) {
        my $day = $1;
        my $id = $2;
        my $cluster = $3;
        my $jobId = $4;
        my $user = $5;

        if ( $cluster eq 'woody' && $day eq $Tday  ) {
            $startedJob{$id} = {
                'day' => $day,
                'cluster' => $cluster,
                'jobId' => $jobId,
                'user' => $user
            };
        }
    }
    if ( /Oct ([0-9]+) .*archiving job... \(dbid: ([0-9]+)\): cluster=([a-z]+), jobId=([0-9]+), user=([a-z0-9]+),/ ) {
        my $day = $1;
        my $id = $2;
        my $cluster = $3;
        my $jobId = $4;
        my $user = $5;

        if ( $cluster eq 'woody' ) {
            $stoppedJob{$id} = {
                'day' => $day,
                'cluster' => $cluster,
                'jobId' => $jobId,
                'user' => $user
            };
        }
    }
}
close FILE;

my $started = 0;
my $count = 0;
my %users;

foreach my $key (keys %startedJob) {
    $started++;
    if ( not exists $stoppedJob{$key} ) {
        $count++;

        if ( not exists $users{$startedJob{$key}->{'user'}} ) {
            $users{$startedJob{$key}->{'user'}} = 1;
        } else {
            $users{$startedJob{$key}->{'user'}}++;
        }

        print <<END;
======
jobID:  $startedJob{$key}->{'jobId'} User:  $startedJob{$key}->{'user'}
======
END
    }
}

foreach my $key ( keys %users ) {
    print "$key => $users{$key}\n";
}

print "Not stopped: $count of $started\n";
