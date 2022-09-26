#!/usr/bin/env perl
use strict;
use warnings;
use utf8;

use JSON::PP; # from Perl default install
use Time::Local qw( timelocal ); # from Perl default install
use Time::Piece; # from Perl default install

### JSON
my $json = JSON::PP->new->allow_nonref;

### TIME AND DATE
# now
my $localtime = localtime;
my $epochtime = $localtime->epoch;
# 5 days ago: Via epoch due to possible reverse month borders
my $epochlessfive = $epochtime - (86400 * 5);
my $locallessfive = localtime($epochlessfive);
# Calc like `date --date 'TZ="Europe/Berlin" 0:00 5 days ago' +%s`)
my ($day, $month, $year) = ($locallessfive->mday, $locallessfive->_mon, $locallessfive->year);
my $checkpointStart = timelocal(0, 0, 0, $day, $month, $year);
# for checkpoints
my $halfday = 43200;

### JOB-ARCHIVE
my $archiveTarget = './cc-backend/var/job-archive';
my $archiveSrc = './source-data/job-archive-source';
my @ArchiveClusters;

# Gen folder
if ( not -d $archiveTarget ){
    mkdir( $archiveTarget ) or die "Couldn't create $archiveTarget directory, $!";
}

# Get clusters by job-archive/$subfolder
opendir my $dh, $archiveSrc  or die "can't open directory: $!";
while ( readdir $dh ) {
    chomp; next if $_ eq '.' or $_ eq '..'  or $_ eq 'job-archive';
    my $cluster = $_;
    push @ArchiveClusters, $cluster;
}

# start for jobarchive
foreach my $cluster ( @ArchiveClusters ) {
    print "Starting to update start- and stoptimes in job-archive for $cluster\n";

    my $clusterTarget = "$archiveTarget/$cluster";

    if ( not -d $clusterTarget ){
        mkdir( $clusterTarget ) or die "Couldn't create $clusterTarget directory, $!";
    }

    opendir my $dhLevel1, "$archiveSrc/$cluster" or die "can't open directory: $!";
    while ( readdir $dhLevel1 ) {
        chomp; next if $_ eq '.' or $_ eq '..';
        my $level1 = $_;

        if ( -d "$archiveSrc/$cluster/$level1" ) {
            opendir my $dhLevel2, "$archiveSrc/$cluster/$level1" or die "can't open directory: $!";
            while ( readdir $dhLevel2 ) {
                chomp; next if $_ eq '.' or $_ eq '..';
                my $level2 = $_;
                my $jobSource = "$archiveSrc/$cluster/$level1/$level2";
                my $jobOrigin = "$jobSource";
                my $jobTargetL1 = "$clusterTarget/$level1";
                my $jobTargetL2 = "$jobTargetL1/$level2";

                # check if files are directly accessible (old format) else get subfolders as file and update path
                if ( ! -e "$jobSource/meta.json") {
                    opendir(D, "$jobSource") || die "Can't open directory $jobSource: $!\n";
                    my @folders = readdir(D);
                    closedir(D);
                    if (!@folders) {
                        next;
                    }

                    foreach my $folder ( @folders ) {
                        next if $folder eq '.' or $folder eq '..';
                        $jobSource = "$jobSource/".$folder;
                    }
                }
                # check if subfolder contains file, else skip
                if ( ! -e "$jobSource/meta.json") {
                    print "$jobSource skipped\n";
                    next;
                }

                open my $metafh, '<', "$jobSource/meta.json" or die "Can't open file $!";
                my $rawstr = do { local $/; <$metafh> };
                close($metafh);
                my $metadata = $json->decode($rawstr);

                # NOTE Start meta.json iteration here
                # my $random_number = int(rand(UPPERLIMIT)) + LOWERLIMIT;
                # Set new startTime: Between 5 days and 1 day before now

                #  Remove id from attributes
                $metadata->{startTime} = $epochtime - (int(rand(432000)) + 86400);
                $metadata->{stopTime} = $metadata->{startTime} + $metadata->{duration};

                # Add starttime subfolder to target path
                my $jobTargetL3 = "$jobTargetL2/".$metadata->{startTime};

                if ( not -d $jobTargetL1 ){
                    mkdir( $jobTargetL1 ) or die "Couldn't create $jobTargetL1 directory, $!";
                }

                if ( not -d $jobTargetL2 ){
                    mkdir( $jobTargetL2 ) or die "Couldn't create $jobTargetL2 directory, $!";
                }

                # target is not directory
                if ( not -d $jobTargetL3 ){
                    mkdir( $jobTargetL3 ) or die "Couldn't create $jobTargetL3 directory, $!";

                    my $outstr = $json->encode($metadata);
                    open my $metaout, '>', "$jobTargetL3/meta.json" or die "Can't write to file $!";
                    print $metaout $outstr;
                    close($metaout);

                    open my $datafh, '<', "$jobSource/data.json" or die "Can't open file $!";
                    my $datastr = do { local $/; <$datafh> };
                    close($datafh);

                    open my $dataout, '>', "$jobTargetL3/data.json" or die "Can't write to file $!";
                    print $dataout $datastr;
                    close($dataout);
                }
            }
        }
    }
}
print "Done for job-archive\n";
sleep(1);
exit;

## CHECKPOINTS
my $checkpTarget = './cc-metric-store/var/checkpoints';
my $checkpSource = './source-data/cc-metric-store-source/checkpoints';
my @CheckpClusters;

# Gen folder
if ( not -d $checkpTarget ){
    mkdir( $checkpTarget ) or die "Couldn't create $checkpTarget directory, $!";
}

# Get clusters by cc-metric-store/$subfolder
opendir my $dhc, $checkpSource  or die "can't open directory: $!";
while ( readdir $dhc ) {
    chomp; next if $_ eq '.' or $_ eq '..'  or $_ eq 'job-archive';
    my $cluster = $_;
    push @CheckpClusters, $cluster;
}
closedir($dhc);

# start for checkpoints
foreach my $cluster ( @CheckpClusters ) {
    print "Starting to update checkpoint filenames and data starttimes for $cluster\n";

    my $clusterTarget = "$checkpTarget/$cluster";

    if ( not -d $clusterTarget ){
        mkdir( $clusterTarget ) or die "Couldn't create $clusterTarget directory, $!";
    }

    opendir my $dhLevel1, "$checkpSource/$cluster" or die "can't open directory: $!";
    while ( readdir $dhLevel1 ) {
        chomp; next if $_ eq '.' or $_ eq '..';
        # Nodename as level1-folder
        my $level1 = $_;

        if ( -d "$checkpSource/$cluster/$level1" ) {

            my $nodeSource = "$checkpSource/$cluster/$level1/";
            my $nodeOrigin = "$nodeSource";
            my $nodeTarget = "$clusterTarget/$level1";
            my @files;

            if ( -e "$nodeSource/1609459200.json") { # 1609459200 == First Checkpoint time in latest dump
                opendir(D, "$nodeSource") || die "Can't open directory $nodeSource: $!\n";
                while ( readdir D ) {
                    chomp; next if $_ eq '.' or $_ eq '..';
                    my $nodeFile = $_;
                    push @files, $nodeFile;
                }
                closedir(D);
                my $length = @files;
                if (!@files || $length != 14) { # needs 14 files == 7 days worth of data
                    next;
                }
            } else {
                next;
            }

            # sort for integer timestamp-filename-part (moduleless): Guarantees start with index == 0 == 1609459200.json
            my @sortedFiles = sort { ($a =~ /^([0-9]{10}).json$/)[0] <=> ($b =~ /^([0-9]{10}).json$/)[0] } @files;

            if ( not -d $nodeTarget ){
                mkdir( $nodeTarget ) or die "Couldn't create $nodeTarget directory, $!";

                while (my ($index, $file) = each(@sortedFiles)) {
                    open my $checkfh, '<', "$nodeSource/$file" or die "Can't open file $!";
                    my $rawstr = do { local $/; <$checkfh> };
                    close($checkfh);
                    my $checkpdata = $json->decode($rawstr);

                    my $newTimestamp = $checkpointStart + ($index * $halfday);
                    # Get Diff from old Timestamp
                    my $timeDiff = $newTimestamp - $checkpdata->{from};
                    # Set new timestamp
                    $checkpdata->{from} = $newTimestamp;

                    foreach my $metric (keys %{$checkpdata->{metrics}}) {
                        $checkpdata->{metrics}->{$metric}->{start} += $timeDiff;
                    }

                    my $outstr = $json->encode($checkpdata);

                    open my $checkout, '>', "$nodeTarget/$newTimestamp.json" or die "Can't write to file $!";
                    print $checkout $outstr;
                    close($checkout);
                }
            }
        }
    }
    closedir($dhLevel1);
}
print "Done for checkpoints\n";
