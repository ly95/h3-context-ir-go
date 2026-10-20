# Independent role structure evaluation

`evaluate_roles.py` reads `fixtures/role_briefs.json`, the separate held-out `fixtures/role_expectations.json`, and an archived complete role run's `report.json`. It makes no provider calls. Original inputs and expected fields are never supplied to a model by this evaluator.

```sh
python conformance/evaluate_roles.py \
  --run conformance/reports/live-roles-round1 \
  --label unique-attempt
python -m unittest discover -s conformance -p test_evaluate_roles.py
```

The output is a new `conformance/reports/roles-heldout-<label>.json`. Existing reports cannot be overwritten. The default `--gate original` exits unsuccessfully if any original field differs, including a documented adapter difference. `--gate native` is an explicit alternative that permits only individually recorded representation corrections; it still rejects every mode, role, retention, copy/property, missing placement/use and binding difference. Neither gate proves prose semantics or video/audio fidelity.

Each result separates:

- `actual_status`: the compiler lifecycle result, including failed cases without artifacts.
- `original_field_status`: exact equality with the independent original fields.
- `native_contract_status`: structural checks after separately recording permitted representation changes.
- `semantic_status`: always `not_determined_by_structural_evaluator`; independent human review is still required.

The checks include the prepared versus final target/assets, exact frame/canvas/fps, timeline endpoints, each source responsibility and retention, waveform copying versus property reference, socket, anchor and containing shot, reciprocal voice binding, and complete Document-to-Artifact target-use records. It rejects camera/storyboard IDs used as narrative locations and observed actors imported from property sources. It verifies that preserved or placed subjects have actual typed narrative/event/speaker uses in their required shots; declaring `shot_ids` or naming someone in literal prose does not count. A replaced figure contributes transfer metadata and must not reappear as its former visible actor.

The original automatic shot count may differ from the production model's unpinned allocation. An original subject scoped to every shot must therefore remain scoped and actually used in every final shot. The evaluator does not keyword-match text to infer actor identity or action meaning. For example, a typed environment used grammatically as a moving car still needs human semantic review even if the IDs and appearances are structurally valid.

Three adapter representations are kept explicit, with the original check still marked unequal:

- `frame_duration_floor_to_nearest`: source truncation at 5166 ms versus native frame-derived rounding at 5167 ms. The native value must match the exact calculation; there is no general ±1 ms tolerance.
- `base_checkpoint_socket_correction`: an explicitly requested original base anchor was wired to `ref_image_N`; the native base checkpoint requires `first_frame` or `last_frame`. An unexpected mode or role change cannot use this exception.
- `base_identity_carried_by_actual_frame`: a fully preserved actual base-frame anchor carries source identity/pose instead of a separate source-subject definition. This does not establish that literal prose or a generated frame depicts that identity correctly.

The sixteen source role Cards are synthetic upstream test inputs. These runs do not demonstrate real video/audio observation or actual rendering. The ca28 historical run's `compiled` cases still contain semantic failures and ambiguities documented separately in `manual-semantic-review-20261020T060925Z-ca28d6982f84.md`.
