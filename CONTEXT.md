# Advertising management

The language used to manage advertising across platforms with adsctl.

## Language

**Advertising platform**:
A provider through which advertisers manage and deliver advertising, such as Meta or Google.
_Avoid_: Channel, network when referring to the provider as a whole.

**Ad account**:
A platform-specific account under which an advertiser manages advertising and its associated billing context.
_Avoid_: Account without qualification, user account.

**Meta campaign**:
A grouping of Meta ad sets organized around an advertising objective.

**Meta ad set**:
A grouping of Meta ads with shared delivery settings, such as targeting, scheduling, and optimization.
_Avoid_: Ad group when referring to Meta.

**Meta ad**:
An advertising entity within a Meta ad set that references an ad creative.

**Meta ad creative**:
The content and presentation used by a Meta ad, including its media and messaging.
_Avoid_: Ad when referring only to the creative content.

**Configured status**:
The status requested for an advertising entity, such as active or paused.
_Avoid_: Delivery status when referring only to the requested setting.

**Effective status**:
The platform-reported operating status of an advertising entity, incorporating conditions such as parent status or review state.

**Insights**:
Performance measurements for advertising entities over a reporting period, interpreted with the selected dimensions and attribution settings.
_Avoid_: Configuration when referring to performance measurements.

**Reporting period**:
The date interval whose advertising performance is being measured.

**Attribution setting**:
The rules that determine which advertising interactions receive credit for a measured action.

**Action type**:
A platform-defined category of measured behavior, such as a link click or purchase.
_Avoid_: Conversion without identifying the relevant behavior when interpreting performance.

**Delivery issue**:
A platform-reported condition that prevents or affects advertising delivery.
_Avoid_: API error when the request succeeded and returned an advertising issue.
