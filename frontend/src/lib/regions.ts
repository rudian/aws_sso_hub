export const AWS_REGIONS: string[] = [
  'us-east-1', 'us-east-2', 'us-west-1', 'us-west-2', 'af-south-1', 'ap-east-1', 'ap-east-2', 'ap-south-1', 'ap-south-2',
  'ap-southeast-1', 'ap-southeast-2', 'ap-southeast-3', 'ap-southeast-4', 'ap-southeast-5', 'ap-southeast-6', 'ap-southeast-7',
  'ap-northeast-1', 'ap-northeast-2', 'ap-northeast-3', 'ca-central-1', 'ca-west-1', 'eu-central-1', 'eu-central-2',
  'eu-west-1', 'eu-west-2', 'eu-west-3', 'eu-south-1', 'eu-south-2', 'eu-north-1', 'il-central-1', 'me-south-1',
  'me-central-1', 'mx-central-1', 'sa-east-1', 'us-gov-west-1', 'us-gov-east-1', 'cn-north-1', 'cn-northwest-1',
] // May not list every region; free text is accepted through <datalist>.

/** Same as awsconfig.RegionRE in the Go backend. */
export const REGION_RE = /^[a-z]{2}(-gov|-iso[a-z]*)?-[a-z]+-[0-9]{1,2}$/
