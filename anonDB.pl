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
    WHERE job_id=?;
    });

my $sth_update_job = $dbh->prepare(qq{
    UPDATE job
    SET user_id = ?,
        project_id = ?
    WHERE id=?;
    });

my $user_index = 0; my $project_index = 0;
my %user_lookup; my %project_lookup;
my %user_group;
my %row;

# build lookups
$sth_select_all->execute;
$sth_select_all->bind_columns( \( @row{ @{$sth_select_all->{NAME_lc} } } ));

while ($sth_select_all->fetch) {
    my $user_id = $row{'user_id'};
    my $project_id = $row{'project_id'};

    if ( not exists $user_lookup{$user_id}) {
        $user_index++;
        $user_lookup{$user_id} = $user_index;
        $user_group{$user_id} = $project_id;
    }

    if ( not exists $project_lookup{$project_id}) {
        $project_index++;
        $project_lookup{$project_id} = $project_index;
    }
}

write_file("user-conversion.json", encode_json \%user_lookup);
write_file("project-conversion.json", encode_json \%project_lookup);
print "$user_index total users\n";
print "$project_index total projects\n";

# convert database
$sth_select_all->execute;
$sth_select_all->bind_columns( \( @row{ @{$sth_select_all->{NAME_lc} } } ));

while ($sth_select_all->fetch) {
    my $user_id = 'user_'.$user_lookup{$row{'user_id'}};
    my $project_id = 'project_'.$project_lookup{$row{'project_id'}};

    # print "$row{'id'}: $user_id - $project_id\n";

    $sth_update_job->execute(
        $user_id,
        $project_id,
        $row{'id'}
    );
}

open(my $fh, '<:encoding(UTF-8)', './jobIds.txt')
    or die "Could not open file  $!";

# convert job meta file
while ( <$fh> ) {

    my $line = $_;
    my ($jobID, $path1, $path2) = split ' ', $line;

    my $json = read_file("$basedir/$path1/$path2/meta.json");
    my $job = decode_json $json;

    my $user = $job->{'user_id'};

    # if ( $user =~ /^user_.*/ ) {
    #     print "$jobID $user\n";
    # }

    my $project;

    if ( exists $user_lookup{$user}) {
        $project = $user_group{$user};
        $user = 'user_'.$user_lookup{$user};
    } else {
        die "$user not in lookup hash!\n";
    }

    if ( exists $project_lookup{$project}) {
        $project = 'project_'.$project_lookup{$project};
    } else {
        die "$project not in lookup hash!\n";
    }

    $job->{user_id} = $user;
    $job->{project_id} = $project;
    $json = encode_json $job;
    write_file("$basedir/$path1/$path2/meta.json", $json);
}
close $fh;

$dbh->disconnect;
