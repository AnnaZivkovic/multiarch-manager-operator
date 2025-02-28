# Considerations for Cluster-wide Weights
# This needs more work

To implement cluster-wide weights, we will use the
`pod.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution` field.

## Merging Methodology

### Append

In the append mode, we concatenate the list of preferred affinity terms in the Cluster Pod Placement Configuration (
CPPC) with those in the Pod being reconciled. This approach allows users to freely set weights in both their pods and
CPPC according to their preferences.
$$ A \mathbin{\|} B \ or \ A \cup B  $$

### Cartesian Product and Normalize

To use the Cartesian product, we must also adjust the weights to ensure balance with the newly added terms.

The steps are:

1. Combine the two sets using the Cartesian product.
2. Append the CPPC preferences as they are.

### Preferred Affinity Terms

Each preferred term is represented as a tuple:

$$
T = (\text{weight} \in \mathbb{N}^+, \text{labelSelectorPredicates})
$$

The resulting set of preferred affinity terms should be the union of the Cartesian product of the pod terms with the
CPPC terms and the CPPC terms themselves:

$$
\text{PodPreferredAffinityTerms}_{\text{New}} = \text{PodPreferredAffinityTerms}_{\text{Old}} \times \text{CPPCPreferredAffinityTerms} \cup \text{CPPCPreferredAffinityTerms}
$$

The maximum possible size of the new preferred affinity terms set is:

$$
|\text{PodPreferredAffinityTerms}_{\text{New}}| = |\text{PodPreferredAffinityTerms}_{\text{Old}} \times \text{CPPCPreferredAffinityTerms} \cup \text{CPPCPreferredAffinityTerms}| \leq |\text{PodPreferredAffinityTerms}_{\text{Old}}| \cdot 4 + 4
$$

By using the Cartesian product, we scale the preference for a given term to `N` preferences weighted by both the
architecture preference and the existing weight.
For example, if the architecture preferences are `{(25, arm64), (75, amd64)}`, and a pod expresses a preference for
nodes labeled `disktype=ssd` with weight 1, the preference will be scaled as follows:

1. `disktype==ssd` + `amd64`
2. `disktype==ssd` + `arm64`
3. `amd64` (includes cases where no `disktype` label is present)
4. `arm64` (includes cases where no `disktype` label is present)

This ensures that:

- The previously set preferred affinity term is safe to modify, as architecture labels are always available.
- Nodes that lack specific labels in the pod’s preferred affinity terms are still scored based on architecture labels.

The issue with this approach is that it can disrupt the existing weight distribution in certain scenarios.

For example, the **Cartesian Merging Strategy** produces:

```yaml
PodPreferredAffinityTerms_new = {
  (31, {amd64, ssd}),
  (77, {arm64, ssd}),
  (22, {amd64}),
  (68, {arm64})
}
```

If there are no available `arm64` nodes, the scheduler will heavily favor the `{amd64, ssd}` rule, likely deprioritizing
any other architectures that also have `ssd`. This imbalance could lead to unintended scheduling preferences.
If we do not have any `arm64` nodes then we are strongly preferring the amd `ssd` rule and will most likely ignore any
other arch that has ssd.

Additionally, a pod requesting `{ssd, arm64}` will be disproportionately skewed because it matches both high-weight
rules, further tilting the scheduling balance.

These are problems that can be avoided by using the append methods.

## **Normalization Formula**

In cases where predefined `PreferredSchedulingTerms` exist, we must examine weather or not to preserve user-defined
weights while incorporating architecture weights without unbalancing them.
We could achieve this using the using normalization.
However, we must be careful as the weights
in [PreferredDuringSchedulingIgnoredDuringExecution](https://github.com/openshift/kubernetes/blob/4683c1b0f6cd62add9fa3469c58fce1b971f48b6/pkg/scheduler/framework/plugins/helper/normalize_score.go#L27)
are normalized.
Here are a few normalizations options. We could try applying these formulas to all the weights or just the existing
weighs or just the arch weighs.

1. $$
   new\_weight = 100 \times \frac{old\_weight}{\sum(arch\_weights)}
   $$
2. $$
   new\_weight = 100 \times \frac{old\_weight}{\sum(existing\_weights)}
   $$
3. $$
   new\_weight = 100 \times \frac{old\_weight}{\sum(all\_weights)}
   $$

#### Issues with Normalization

When applying any of these normalization functions to all weights, the final affinity remains unchanged, making the
operation ineffective.

Since Kubernetes already normalizes predefined weights, we could normalize CPPC node affinities to match predefined
ones. However, this approach may unintentionally skew preferences toward the CPPC-defined architectures.

### Scale Factors

Insted of normlizing our valuse we could try using a scale factor to adjust them.

1. $$
   new\_weight = \frac{old\_weight \times \sum(arch\_weights)}{\sum(all\_weights)}
   $$
2. $$
   new\_weight = \frac{old\_weight \times \sum(existing\_weights)}{\sum(all\_weights)}
   $$
3. $$
   new\_weight = \frac{old\_weight \times \sum(arch\_weights)}{\sum(existing\_weights)}
   $$
4. $$
   new\_weight = \frac{old\_weight \times \sum(existing\_weights)}{\sum(arch\_weights)}
   $$

#### Observations on Scale Factors

Initial investigations suggest scale factors may produce better results. However, unwanted behaviors may still arise:

If architecture values are significantly larger than existing values, options 3 and 4 may result in a much
stronger/weaker affinity.

Options 1 and 2 appear more promising and require further study..

## **Final Thoughts**

The append strategy appears to be the best approach due to the downsides of the Cartesian product.

Further analysis is needed to determine whether normalization provides tangible benefits or unnecessarily complicates
scheduling logic. Additionally, scale factors might be a more effective solution, but further investigation is
necessary.

At this stage, we are implementing the simplest use case: appending to `PreferredDuringSchedulingIgnoredDuringExecution`
without normalization or scaling. This ensures maximum predictability for users when predefined terms are present. If no
additional architectures are specified, no modifications will be made.are spifided no acctions will be taken. 